package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type WebSite struct {
	client    *http.Client
	baseURL   string
	verbose   bool
	userAgent string
}

func NewWebSite(verbose bool) (*WebSite, error) {
	verbose = true
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("error creating cookie jar: %w", err)
	}

	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	return &WebSite{
		client:    client,
		baseURL:   "https://www.vuokraovi.com",
		verbose:   verbose,
		userAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
	}, nil
}

func (w *WebSite) logRequest(method, url string) {
	if w.verbose {
		log.Printf("[%s] %s", method, url)
	}
}

func (w *WebSite) FetchRentalOffers(formData string, maxPages int) ([]RentalOffer, error) {
	initialURL := "https://www.vuokraovi.com/haku/vuokra-asunnot?locale=fi"
	if w.verbose {
		log.Printf("Sending initial POST request to %s", initialURL)
	}

	offers, nextPageURL, err := w.fetchAndParse(initialURL, "POST", formData)
	if err != nil {
		return nil, fmt.Errorf("error fetching initial page: %w", err)
	}

	allOffers := offers

	// Follow pagination links until the end or until max pages is reached
	pageNum := 2
	for nextPageURL != "" {
		// Check if we've reached the maximum number of pages
		if maxPages > 0 && pageNum > maxPages {
			if w.verbose {
				log.Printf("Reached maximum number of pages (%d). Stopping pagination.", maxPages)
			}
			break
		}

		if w.verbose {
			log.Printf("Fetching page %d: %s", pageNum, nextPageURL)
		}

		pageOffers, newNextPageURL, err := w.fetchAndParse(nextPageURL, "GET", "")
		if err != nil {
			log.Printf("Error fetching page %d: %v", pageNum, err)
			break
		}

		allOffers = append(allOffers, pageOffers...)
		nextPageURL = newNextPageURL
		pageNum++

		// Add a small delay between requests to be nice to the server
		time.Sleep(500 * time.Millisecond)
	}

	return allOffers, nil
}

func (w *WebSite) fetchAndParse(targetURL, method, formData string) ([]RentalOffer, string, error) {
	w.logRequest(method, targetURL)

	var req *http.Request
	var err error

	if method == "POST" {
		req, err = http.NewRequest("POST", targetURL, bytes.NewBufferString(formData))
		if err != nil {
			return nil, "", fmt.Errorf("error creating POST request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req, err = http.NewRequest("GET", targetURL, nil)
		if err != nil {
			return nil, "", fmt.Errorf("error creating GET request: %w", err)
		}
	}

	// Set common headers
	req.Header.Set("User-Agent", w.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	// Send the request
	resp, err := w.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("error reading response body: %w", err)
	}

	// Parse the HTML document
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, "", fmt.Errorf("error parsing HTML: %w", err)
	}

	// Extract rental offers using the function from parser.go
	offers := extractRentalOffers(doc, w.baseURL)

	if w.verbose {
		log.Printf("Found %d offers on current page", len(offers))
	}

	// Check for pagination link
	nextPageURL := ""
	doc.Find("link[rel='next']").Each(func(i int, s *goquery.Selection) {
		if href, exists := s.Attr("href"); exists {
			if !strings.HasPrefix(href, "http") {
				href = w.baseURL + href
			}
			nextPageURL = href
		}
	})

	return offers, nextPageURL, nil
}
