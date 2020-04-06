package main

import (
	"bufio"
	"flag"
	"fmt"
	"golang.org/x/net/html"
	"gopkg.in/matryer/try.v1"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
)

func imageNameFromUrl(imageUrl string) string {
	fileUrl, err := url.Parse(imageUrl)
	if err != nil {
		fmt.Printf("ERR - Failed to parse url to get image name from url %s. Error: %v\n", imageUrl, err)
		return ""
	}

	segments := strings.Split(fileUrl.Path, "/")
	return segments[len(segments)-1]
}

func downloadImage(directory string, url string, chDone chan bool) {
	defer func() { chDone <- true }()

	err := try.Do(func(attempt int) (bool, error) {
		if attempt > 1 {
			fmt.Printf("INFO - Attempt #%d on image %s!\n", attempt, url)
		}

		resp, err := http.Get(url)
		if err != nil {
			fmt.Printf("ERR - Failed to get page from image url %s. Error is %v. Retrying...\n", url, err)
			return true, err
		}

		defer resp.Body.Close()

		file, err := os.Create(directory + "/" + imageNameFromUrl(url))
		if err != nil {
			fmt.Printf("ERR - Failed to create image file from image url %s. Error is %v. Retrying...\n", url, err)
			return true, err
		}
		defer file.Close()

		written, err := io.Copy(file, resp.Body)
		if err != nil {
			fmt.Printf("ERR - Failed to save image from url %s to file. Error is %v. Retrying...\n", url, err)
			return true, err
		}

		downloadedBytes, _ := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
		if written != downloadedBytes {
			fmt.Printf("ERR - Content length mismatch between written bytes and downloaded bytes! Retrying...\n")
			return true, err
		}

		return false, nil
	})

	if err != nil {
		fmt.Printf("INFO - Gave up downloading image %s after %d retries. Fuck that image in particular.\n", url, try.MaxRetries)
	}
}

func crawlAlbumImages(url string, chTitle chan string, chImages chan string, chDone chan bool) {
	defer func() { chDone <- true }()

	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("ERR - Failed to get page from url %s. Error: %v\n", url, err)
		return
	}

	defer resp.Body.Close()
	z := html.NewTokenizer(resp.Body)

	getTokenAttrValue := func(t html.Token, attrName string) (value string, present bool) {
		for _, a := range t.Attr {
			if a.Key == attrName {
				value = a.Val
				present = true
				return
			}
		}
		return
	}

	for {
		switch z.Next() {
		case html.ErrorToken:
			return
		case html.StartTagToken:
			t := z.Token()
			switch t.Data {
			case "a":
				class, present := getTokenAttrValue(t, "class")
				if !present || class != "image" {
					continue
				}

				href, present := getTokenAttrValue(t, "href")
				if !present || !strings.Contains(href, "http") {
					continue
				}

				chImages <- href
				break
			case "h1":
				id, present := getTokenAttrValue(t, "id")
				if !present || id != "title" {
					continue
				}
				if z.Next() == html.TextToken {
					title := strings.TrimSpace(z.Token().Data)
					chTitle <- title
				}
			}
		}
	}
}

func downloadAlbum(url string, wg *sync.WaitGroup) {
	defer wg.Done()

	chTitle := make(chan string)
	chImages := make(chan string)
	chUrlsCrawlDone := make(chan bool)

	var title string
	var images []string
	go crawlAlbumImages(url, chTitle, chImages, chUrlsCrawlDone)

crawl:
	for {
		select {
		case tit := <-chTitle:
			title = tit
		case img := <-chImages:
			images = append(images, img)
		case <-chUrlsCrawlDone:
			break crawl
		}
	}

	fmt.Printf("Fetched album '%s'! Downloading it...\n", title)
	directory := title

	if _, err := os.Stat(directory); os.IsNotExist(err) {
		err = os.Mkdir(directory, 0755)
		if err != nil {
			fmt.Printf("ERR - Failed to create directory %s/. Error: %v\n", directory, err)
			return
		}
	}

	chDoneDownload := make(chan bool)
	for _, image := range images {
		go downloadImage(directory, image, chDoneDownload)
	}

	for c := range images {
		select {
		case <-chDoneDownload:
			c++
		}
	}

	fmt.Printf("INFO - Downloaded the entire '%s' album! (%d images)\n", title, len(images))
}

func downloadAlbums(urls []string) {
	var wg sync.WaitGroup

	for _, album := range urls {
		wg.Add(1)
		go downloadAlbum(album, &wg)
	}

	wg.Wait()
}

func main() {
	multiple := flag.Bool("m", false,
		"True if you want to download multiple albums by passing a text file containing album links as input.")
	batchSize := flag.Int("batchSize", 5,
		"Number of concurrent allowed album downloads (default is 5).")
	flag.Parse()

	if !*multiple {
		albumUrl := os.Args[1]

		var wg sync.WaitGroup
		wg.Add(1)
		go downloadAlbum(albumUrl, &wg)

		wg.Wait()
	}

	if *multiple {
		albumsFile := os.Args[2]

		file, err := os.Open(albumsFile)
		if err != nil {
			log.Fatal(err)
		}

		var albums []string
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			albums = append(albums, scanner.Text())
		}

		segments := int64(math.Ceil(float64(len(albums) / *batchSize)))
		idx := 0
		fmt.Printf("Ready to download %d albums! I will divide them into %d segments so I don't hit rate limit on Cyberdrop. Go!\n", len(albums), segments)

		for idx < len(albums) {
			queue := albums[idx:(idx + *batchSize)]
			downloadAlbums(queue)
			idx += *batchSize
		}
	}
}
