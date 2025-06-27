package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/cdproto/network"
)

func sanitizeFileName(label string) string {
	if strings.Contains(label, "Play epic song") {
		return "epic-song.mp4"
	}
	label = strings.ToLower(label)
	label = strings.ReplaceAll(label, "play ", "")
	label = strings.ReplaceAll(label, " ", "-")
	reg := regexp.MustCompile(`[^a-z0-9\-]+`)
	label = reg.ReplaceAllString(label, "")
	return label + ".mp4"
}

func downloadFile(url, filename string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

type divMeta struct {
	ID        string
	AriaLabel string
}

func main() {
	var url string
	flag.StringVar(&url, "u", "", "URL to visit")
	flag.Parse()
	if url == "" {
		log.Fatal("Must provide -u URL")
	}

	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	if err := chromedp.Run(ctx, network.Enable()); err != nil {
		log.Fatalf("Enable network events: %v", err)
	}

	requests := make(chan string, 100)

	// Set up the listener ONCE before the loop, no removal/cancellation after
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		if e, ok := ev.(*network.EventRequestWillBeSent); ok {
			if strings.Contains(e.Request.URL, "t4.bcbits.com") {
				select {
				case requests <- e.Request.URL:
				default:
					// Drop if channel overflow
				}
			}
		}
	})

	var divs []divMeta
	err := chromedp.Run(ctx, chromedp.Tasks{
		chromedp.Navigate(url),
		chromedp.Sleep(2 * time.Second),
		chromedp.EvaluateAsDevTools(`
			(() => {
				const out = [];
				const divs = Array.from(document.querySelectorAll("div.play_status"));
				divs.forEach((el,i) => {
					el.setAttribute("data-ccid", "play"+i);
					let anchor = el.closest("a");
					let aria = anchor ? anchor.getAttribute("aria-label") : "";
					out.push({id:"play"+i, ariaLabel: aria ? aria : ""});
				});
				return out;
			})()
		`, &divs),
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Found %d play_status divs\n", len(divs))

	for _, info := range divs {
		filename := sanitizeFileName(info.AriaLabel)
		fmt.Printf("Will download to: %s (aria-label: %q)\n", filename, info.AriaLabel)

		clickTask := chromedp.Tasks{
			chromedp.Evaluate(fmt.Sprintf(
				`document.querySelector('[data-ccid="%s"]').click()`, info.ID),
				nil,
			),
			chromedp.Sleep(1 * time.Second),
		}
		if err := chromedp.Run(ctx, clickTask); err != nil {
			log.Printf("Error clicking %s: %v", info.ID, err)
		}

		var audioURL string
		timeout := time.After(2 * time.Second)
		found := false
		for !found {
			select {
			case url := <-requests:
				audioURL = url
				found = true
			case <-timeout:
				fmt.Printf("Timeout waiting for t4.bcbits.com request for div %s\n", info.ID)
				found = true
			}
		}
		if audioURL != "" {
			err = downloadFile(audioURL, filename)
			if err != nil {
				fmt.Printf("Download error for %s: %v\n", audioURL, err)
			} else {
				fmt.Printf("Downloaded %s\n", filename)
			}
		}
	}
}
