package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/fatih/color"
)

// RentalOffer represents a rental property listing
// This should match the definition in parser.go
type RentalOffer struct {
	Title     string
	Address   string
	Price     string
	Size      string
	Rooms     string
	Available string
	Link      string
}

func main() {
	// Define command-line flags
	maxPagesPtr := flag.Int("limit", 0, "Maximum number of pages to query (0 = no limit)")
	verbosePtr := flag.Bool("verbose", false, "Enable verbose logging")
	formDataFilePtr := flag.String("form", "form_data.txt", "Path to form data file")
	flag.Parse()

	// Set up logging
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Create a cookie jar to store cookies between requests
	jar, err := cookiejar.New(nil)
	if err != nil {
		log.Fatalf("Error creating cookie jar: %v", err)
	}

	// Create HTTP client with cookie jar
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Allow up to 10 redirects
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	// Base URL for the website
	baseURL := "https://www.vuokraovi.com"

	// Read form data from file
	formData, err := os.ReadFile(*formDataFilePtr)
	if err != nil {
		log.Fatalf("Error reading form data from %s: %v", *formDataFilePtr, err)
	}

	// Send initial POST request
	initialURL := "https://www.vuokraovi.com/haku/vuokra-asunnot?locale=fi"
	if *verbosePtr {
		log.Printf("Sending initial POST request to %s", initialURL)
	}

	offers, nextPageURL, err := fetchAndParse(client, initialURL, "POST", string(formData), baseURL, *verbosePtr)
	if err != nil {
		log.Fatalf("Error fetching initial page: %v", err)
	}

	allOffers := offers

	// Follow pagination links until the end or until max pages is reached
	pageNum := 2
	for nextPageURL != "" {
		// Check if we've reached the maximum number of pages
		if *maxPagesPtr > 0 && pageNum > *maxPagesPtr {
			if *verbosePtr {
				log.Printf("Reached maximum number of pages (%d). Stopping pagination.", *maxPagesPtr)
			}
			break
		}

		if *verbosePtr {
			log.Printf("Fetching page %d: %s", pageNum, nextPageURL)
		}

		pageOffers, newNextPageURL, err := fetchAndParse(client, nextPageURL, "GET", "", baseURL, *verbosePtr)
		if err != nil {
			log.Printf("Error fetching page %d: %v", pageNum, err)
			break
		}

		allOffers = append(allOffers, pageOffers...)
		nextPageURL = newNextPageURL
		pageNum++
	}

	// Print results
	printResults(allOffers)
}

// fetchAndParse sends a request to the specified URL and parses the response
func fetchAndParse(client *http.Client, targetURL, method, formData, baseURL string, verbose bool) ([]RentalOffer, string, error) {
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
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	// Send the request
	resp, err := client.Do(req)
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
	offers := extractRentalOffers(doc, baseURL)

	if verbose {
		log.Printf("Found %d offers on current page", len(offers))
	}

	// Check for pagination link
	nextPageURL := ""
	doc.Find("link[rel='next']").Each(func(i int, s *goquery.Selection) {
		if href, exists := s.Attr("href"); exists {
			if !strings.HasPrefix(href, "http") {
				href = baseURL + href
			}
			nextPageURL = href
		}
	})

	return offers, nextPageURL, nil
}

// printResults prints the rental offers to the console
func printResults(offers []RentalOffer) {
	titleColor := color.New(color.FgCyan, color.Bold)
	addressColor := color.New(color.FgYellow)
	priceColor := color.New(color.FgGreen, color.Bold)
	roomsColor := color.New(color.FgMagenta, color.Bold)
	detailsColor := color.New(color.FgWhite)
	linkColor := color.New(color.FgBlue, color.Underline)

	fmt.Printf("\nFound %d rental offers:\n\n", len(offers))

	for i, offer := range offers {
		fmt.Printf("--- Offer #%d ---\n", i+1)

		if offer.Title != "" {
			titleColor.Printf("Title: %s\n", offer.Title)
		}

		if offer.Address != "" {
			addressColor.Printf("Address: %s\n", offer.Address)
		}

		if offer.Price != "" {
			priceColor.Printf("Price: %s\n", offer.Price)
		}

		if offer.Rooms != "" {
			roomsColor.Printf("Rooms: %s\n", offer.Rooms)
		}

		details := []string{}
		if offer.Size != "" {
			details = append(details, "Size: "+offer.Size)
		}
		if offer.Available != "" {
			details = append(details, "Available: "+offer.Available)
		}

		if len(details) > 0 {
			detailsColor.Printf("%s\n", strings.Join(details, " | "))
		}

		if offer.Link != "" {
			linkColor.Printf("Link: %s\n", offer.Link)
		}

		fmt.Println()
	}
}
