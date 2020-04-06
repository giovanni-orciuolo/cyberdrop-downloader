package main

import (
	"fmt"
	"golang.org/x/net/html"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func getTokenAttrValue(t html.Token, attrName string) (value string, present bool) {
	for _, a := range t.Attr {
		if a.Key == attrName {
			value = a.Val
			present = true
			return
		}
	}

	return
}

func imageNameFromUrl(imageUrl string) string {
	fileUrl, err := url.Parse(imageUrl)
	if err != nil {
		fmt.Printf("ERR - Failed to parse url to get image name from url %s. Error: %v\n", imageUrl, err)
		return ""
	}

	segments := strings.Split(fileUrl.Path, "/")
	return segments[len(segments) - 1]
}

func downloadImage(url string, chFile chan *os.File, chDone chan bool) {
	defer func() { chDone <- true }()

	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("ERR - Failed to get page from image url %s. Error: %v\n", url, err)
		return
	}

	defer resp.Body.Close()
	file, err := os.Create(imageNameFromUrl(url))
	if err != nil {
		fmt.Printf("ERR - Failed to create image file from image url %s. Error: %v\n", url, err)
		return
	}

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		fmt.Printf("ERR - Failed to save image from url %s to file. Error: %v\n", url, err)
		return
	}

	chFile <- file
}

func crawlAlbumImages(url string, chUrls chan string, chDone chan bool) {
	defer func() { chDone <- true }()

	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("ERR - Failed to get page from url %s. Error: %v\n", url, err)
		return
	}

	defer resp.Body.Close()
	z := html.NewTokenizer(resp.Body)

	for {
		switch z.Next() {
		case html.ErrorToken: return
		case html.StartTagToken:
			t := z.Token()
			if t.Data != "a" {
				continue
			}

			class, present := getTokenAttrValue(t, "class")
			if !present || class != "image" {
				continue
			}

			href, present := getTokenAttrValue(t, "href")
			if !present || !strings.Contains(href, "http") {
				continue
			}

			chUrls <- href
		}
	}
}

func downloadAlbum(url string, chDone chan bool) {
	defer func() { chDone <- true }()
	chUrls := make(chan string)
	chUrlsCrawlDone := make(chan bool)

	var images []string
	go crawlAlbumImages(url, chUrls, chUrlsCrawlDone)

	crawl:
	for {
		select {
		case img := <-chUrls: images = append(images, img)
		case <-chUrlsCrawlDone: break crawl
		}
	}

	chFile := make(chan *os.File)
	chDoneDownload := make(chan bool)
	for _, image := range images {
		go downloadImage(image, chFile, chDoneDownload)
	}

	var files []*os.File
	for c := range images {
		select {
		case file := <-chFile: files = append(files, file)
		case <-chDoneDownload: c++
		}
	}

	fmt.Printf("Downloaded %d files!\n", len(files))
	for _, file := range files {
		fmt.Printf("%s, ", file.Name())
	}
}

func main() {
	chDone := make(chan bool)

	go downloadAlbum("album_url_here", chDone)
	download:
	for {
		select {
		case <-chDone: break download
		}
	}
}