package main

import (
	// Built-in Go features
	"bufio"
	"flag"
	"fmt"
	"github.com/vbauerster/mpb"
	"github.com/vbauerster/mpb/decor"
	"golang.org/x/net/html"
	"gopkg.in/matryer/try.v1"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

func addProgressBar(p *mpb.Progress, title string, total int) *mpb.Bar {
	return p.AddBar(
		int64(total),
		mpb.PrependDecorators(
			decor.Name(title+":", decor.WC{W: len(title) + 1, C: decor.DidentRight}),
			decor.OnComplete(decor.Name("downloading", decor.WCSyncSpaceR), "done!"),
			decor.OnComplete(decor.CountersNoUnit("%d / %d", decor.WCSyncWidth), ""),
		),
		mpb.AppendDecorators(
			decor.OnComplete(decor.Percentage(decor.WC{W: 5}), ""),
		),
	)
}

func imageNameFromUrl(imageUrl string) (string, error) {
	fileUrl, err := url.Parse(imageUrl)
	if err != nil {
		// fmt.Printf("ERR - Failed to parse url to get image name from url %s. Error: %v\n", imageUrl, err)
		return "", err
	}

	segments := strings.Split(fileUrl.Path, "/")
	return segments[len(segments)-1], nil
}

func downloadImage(directory string, url string, b *mpb.Bar) {
	defer func() {
		b.Increment()
	}()

	err := try.Do(func(attempt int) (bool, error) {
		//if attempt > 1 {
		//	fmt.Printf("INFO - Attempt #%d on image %s!\n", attempt, url)
		//}

		resp, err := http.Get(url)
		if err != nil {
			// fmt.Printf("ERR - Failed to get page from image url %s. Error is %v. Retrying...\n", url, err)
			return true, err
		}
		defer resp.Body.Close()

		imageName, err := imageNameFromUrl(url)
		if err != nil {
			return true, err
		}

		file, err := os.Create(directory + "/" + imageName)
		if err != nil {
			// fmt.Printf("ERR - Failed to create image file from image url %s. Error is %v. Retrying...\n", url, err)
			return true, err
		}
		defer file.Close()

		written, err := io.Copy(file, resp.Body)
		if err != nil {
			// fmt.Printf("ERR - Failed to save image from url %s to file. Error is %v. Retrying...\n", url, err)
			return true, err
		}

		downloadedBytes, _ := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
		if written != downloadedBytes {
			// fmt.Printf("ERR - Content length mismatch between written bytes and downloaded bytes! Retrying...\n")
			return true, err
		}

		return false, nil
	})

	if err != nil {
		// fmt.Printf("INFO - Gave up downloading image %s after %d retries. Fuck that image in particular.\n", url, try.MaxRetries)
		b.Abort(false)
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
				// Automatically gets the best URL from the other anchor
				// Thanks @antiops for the tip!
				id, present := getTokenAttrValue(t, "id")
				if !present || id != "file" {
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

func downloadAlbum(url string, wg *sync.WaitGroup, p *mpb.Progress) {
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

	// fmt.Printf("Fetched album '%s'! Downloading it...\n", title)
	b := addProgressBar(p, title, len(images))

	directory := title
	if _, err := os.Stat(directory); os.IsNotExist(err) {
		err = os.Mkdir(directory, 0755)
		if err != nil {
			b.Abort(false)
			// fmt.Printf("ERR - Failed to create directory %s/. Error: %v\n", directory, err)
			return
		}
	}

	for _, image := range images {
		go downloadImage(directory, image, b)
	}

	for !b.Completed() {
		// Wait for bar completion
	}

	// fmt.Printf("INFO - Downloaded the entire '%s' album! (%d images)\n", title, len(images))
}

func downloadAlbums(urls []string) {
	var wg sync.WaitGroup
	wg.Add(len(urls))

	p := mpb.New(
		mpb.WithWaitGroup(&wg),
		mpb.WithWidth(60),
		mpb.WithRefreshRate(180*time.Millisecond),
	)

	for _, album := range urls {
		go downloadAlbum(album, &wg, p)
	}

	wg.Wait()
}

func main() {
	multiple := flag.Bool("m", false,
		"True if you want to download multiple albums by passing a text file containing album links as input.")
	batchSize := flag.Int("b", 5,
		"Number of concurrent allowed album downloads (default is 5).")
	flag.Parse()

	if !*multiple {
		albumUrl := os.Args[1]

		var wg sync.WaitGroup
		wg.Add(1)

		p := mpb.New(
			mpb.WithWaitGroup(&wg),
			mpb.WithWidth(60),
			mpb.WithRefreshRate(180*time.Millisecond),
		)

		go downloadAlbum(albumUrl, &wg, p)

		wg.Wait()
	}

	if *multiple {
		albumsFile := os.Args[2]

		file, err := os.Open(albumsFile)
		if err != nil {
			log.Fatal(err)
			return
		}

		var albums []string
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			albums = append(albums, scanner.Text())
		}

		if len(albums) == 0 {
			log.Fatal("There are no albums to download!")
			return
		}

		fmt.Printf("Ready to download %d albums!\n", len(albums))
		idx := 0

		for idx < len(albums) {
			// Fuck Go doesn't have ternary expressions!!!
			var queue []string
			if *batchSize < len(albums) {
				queue = albums[idx:(idx + *batchSize)]
			} else {
				queue = albums
			}

			downloadAlbums(queue)
			idx += *batchSize
		}
	}
}
