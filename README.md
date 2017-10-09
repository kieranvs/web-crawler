# web-crawler
Simple web crawler in Go.

This is the first version of a simple web crawler in Go.

## Usage

```
go run crawler.go               //
-workers 5                      // how many simultaneous HTTP requests to perform
-target "http://kieranvs.com"   // base url of target website
-page "/index.html"             // page to start exploring at
```

## Results

The program writes to a file called `output.html`, which draws a simple network graph using `SpringyJS`.

## Local testing

To host the website contained in \local-test:

Modify `server.go` to contain the full address of `\local-test`.

```
go run server.go
```

will host the website at `http://localhost:8080/`.
 
## Notes

The current system for displaying the results does not scale well to super large websites.