package main

import (
	"flag"
	"fmt"
	"net/url"
	"net/http"
	"os"
	"strconv"
	"golang.org/x/net/html"
)

/* Resource represents a page or file */
type resource string;

/* PageLink represents a link from one Resource to another */
type PageLink struct {
	from resource;
	to resource;
}

/* ScrapeTask represents a link which needs to be followed by a worker */
type ScrapeTask struct {
	baseurl string;
	page resource;
	depth int;
}

func main() {
	/* command line arguments */
	worker_count := flag.Int("workers", 3, "Number of concurrent http requests");
	target_base := flag.String("target", "http://localhost:8080", "Target base url e.g. http://website.com");
	target_page := flag.String("page", "/index.html", "Page to start at");

	flag.Parse();

	/* program channels */
	task_submit := make(chan ScrapeTask); //tasks submitted to the worker pool
	task_queue := make(chan ScrapeTask); //tasks waiting to be retrieved by workers
	task_done := make(chan int, 100); //notify on this channel when task is done
	results := make(chan PageLink, 100); //result pagelinks to be processed

	/* program components */
	go unbounded_buffer(task_submit, task_queue, task_done, results);
	for n := 0; n < *worker_count; n++ {
		go scrape_worker(n, task_queue, results, task_submit, task_done)
	}
	go springyjs_printer(results);

	task_submit <- ScrapeTask{baseurl: *target_base, page: resource(*target_page), depth: 0};

	select {}
}

/*
Unbounded queue of ScrapeTasks between input and output.
Removes duplicate tasks for same page.
Keeps track of the number of delegated tasks and closes results channel when done.
*/
func unbounded_buffer(input chan ScrapeTask, output chan ScrapeTask, task_done chan int, results chan PageLink) {
	queue := []ScrapeTask{};
	done := make(map[resource]bool);
	unfinished := 0;
	started := false;

	for {
		if (len(queue) == 0 && unfinished == 0 && started) {
			close(results);
		}
		if (len(queue) == 0) {
			select {
			case d := <- input:
				if (!done[d.page]) {
					done[d.page] = true;
					queue = append(queue, d);
					unfinished += 1;
					started = true;
				}
			case <- task_done:
				unfinished -= 1;
			}
		} else {
			select {
			case d := <- input:
				if (!done[d.page]) {
					done[d.page] = true;
					queue = append(queue, d);
					unfinished += 1;
				}
			case output <- queue[0]:
				queue = queue[1:];
			case <- task_done:
				unfinished -= 1;
			}
		}
	}
}

/*
Scrape Workers take tasks from the task_queue, scrape the page, adding results to results and newly discovered pages to task_submit. Signals on task_done when the task is done to facilitate clean program termination.
*/
func scrape_worker(worker_id int, task_queue chan ScrapeTask, results chan PageLink, task_submit chan ScrapeTask, task_done chan int) {
	for {
		task := <- task_queue;
		if(task.depth < 2) {
			task_status := scrape(task, results, task_submit);
			fmt.Println("Worker", worker_id, ":", task_status, "[", string(task.page), "]");
		}
		task_done <- 0;
	}
}

func scrape(task ScrapeTask, results chan PageLink, task_submit chan ScrapeTask) string {
	newurl := fix_url(string(task.baseurl), string(task.page));

	u, _ := url.Parse(newurl);
	bu, _ := url.Parse(task.baseurl);
	if(u.Host != bu.Host) {
		return "Rejected due to hostname=" + string(u.Host);
	}
	if(u.Scheme != "http" && u.Scheme != "https") {
		return "Rejected due to scheme=" + string(u.Scheme);
	}

	resp, err := http.Get(newurl);
	if err != nil {
    	return "HTTP error";
	}
	contentType := resp.Header.Get("Content-Type");
	if(len(contentType) < 11 || contentType[0:10] != "text/html;") {
		return "Rejected due to content-type=" + contentType;
	}

	z := html.NewTokenizer(resp.Body)

	defer resp.Body.Close()

	for {
	    tt := z.Next()

	    switch {
	    case tt == html.ErrorToken:
	    	return "Done";
	    case tt == html.StartTagToken:
	        t := z.Token()

	        if t.Data == "a" {
	            for _, a := range t.Attr {
				    if a.Key == "href" {
				    	pl := PageLink{from: task.page, to: resource(a.Val)};
				    	st := ScrapeTask{baseurl: task.baseurl, page: resource(a.Val), depth: task.depth + 1};
				    	task_submit <- st;
				        results <- pl;
				        break
				    }
				}
	        }
	        if t.Data == "link" {
	        	for _, a := range t.Attr {
	        		if a.Key == "href" {
	        			pl := PageLink{from: task.page, to: resource(a.Val)};
	        			results <- pl;
	        		}
	        	}
	        }
	        if t.Data == "script" {
	        	for _, a := range t.Attr {
	        		if a.Key == "src" {
	        			pl := PageLink{from: task.page, to: resource(a.Val)};
	        			results <- pl;
	        		}
	        	}
	        }
	    case tt == html.SelfClosingTagToken:
	    	t := z.Token();
	    	if t.Data == "img" {
	        	for _, a := range t.Attr {
	        		if a.Key == "src" {
	        			pl := PageLink{from: task.page, to: resource(a.Val)};
	        			results <- pl;
	        		}
	        	}
	        }
	    }
	}
}

func fix_url(baseurl string, relurl string) string {
	u, _ := url.Parse(relurl)
    base, _ := url.Parse(baseurl)
    return base.ResolveReference(u).String()
}

/* Results consumer for debugging */
func simple_printer(input chan PageLink) {
	for {
		val := <- input;
		fmt.Println(val.from, " -> ", val.to);
	}
}


/*

==================================

Pretty printing in graph form using SpringyJS

springyjs_printer consumes the results and builds a graph.
When the results channel is closed, it writes output.html to draw the graph using SpringyJS.

*/

func contains(value string, list []string) bool {
    for _, v := range list {
        if v == value {
            return true
        }
    }
    return false
}

func insertEdge(from string, to string, list *[]PageLinkEdge) {
    for i, v := range *list {
        if (v.from == from && v.to == to) {
            (*list)[i].count += 1;
            return;
        }
    }
    *list = append(*list, PageLinkEdge{from: from, to: to, count: 1});
}

type PageLinkEdge struct {
	from string;
	to string;
	count int;
}

func springyjs_printer(input chan PageLink) {
	nodes := []string{};
	edges := []PageLinkEdge{};
	for val := range input {
		if(!contains(string(val.from), nodes)) {
			nodes = append(nodes, string(val.from));
		}
		if(!contains(string(val.to), nodes)) {
			nodes = append(nodes, string(val.to));
		}
		insertEdge(string(val.from), string(val.to), &edges);
	}

	fmt.Println("Writing to output.html");
	f, _ := os.Create("/Users/Kieran/Desktop/crawler/output.html");
	f.WriteString("<html>\n<body>\n<script src=\"http://ajax.googleapis.com/ajax/libs/jquery/1.3.2/jquery.min.js\"></script>\n<script src=\"springy.js\"></script>\n<script src=\"springyui.js\"></script>\n<script>\nvar graph = new Springy.Graph();\n");

	for _, n := range nodes {
		f.WriteString("graph.addNodes('" + n + "');\n");
	}

	f.WriteString("graph.addEdges(\n");

	for _, e := range edges {
		f.WriteString("['" + string(e.from) + "', '" + string(e.to) + "'," +
			"{color: '#000000', label: '" + strconv.Itoa(e.count) + "'}" + 
			"],\n");
	}

	f.WriteString(");\n\n");

	f.WriteString("jQuery(function(){\nvar springy = jQuery('#springydemo').springy({\ngraph: graph\n});\n});\n</script>\n<canvas id=\"springydemo\" width=\"1200\" height=\"800\" />\n</body>\n</html>");

	f.Sync();
	f.Close();
	os.Exit(0);
}





