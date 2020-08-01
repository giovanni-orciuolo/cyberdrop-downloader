## Go Cyberdrop Downloader

This is my own Cyberdrop.me album downloader, written in Go purely as an exercise. I'm quite happy with how this
experiment is going, Go is a very interesting language indeed.

### How to use

- Download **single album**
```
$ cyberdrop-downloader https://cyberdrop.me/a/album_code_here
```
- Download **multiple albums**
```
$ cyberdrop-downloader -m albums.txt -b 5
```

#### Flags
-m (multiple): Reference a text file where each row is a Cyberdrop album link.

-b (batchSize): Specify how many albums you want to download simultaneously (default is 5).

### How to build

**Install Go @ https://golang.org if you don't have it already**

First clone this repository:
```
$ git clone https://github.com/DoubleHub/cyberdrop-downloader
```

Then install the following dependencies:
```
$ go get golang.org/x/net/html
$ go get gopkg.in/matryer/try.v1
$ go get github.com/vbauerster/mpb
```

Then you just build it:
```
$ go build main.go
$ mv main cyberdrop-downloader # If you want to rename the executable
```

### TODO
- Better progress bar  (the current one is often misaligned)
- Releases directly on Github (so you don't have to build this yourself)