package main

import (
	"log"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// extractRentalOffers extracts rental offers from the HTML document
func extractRentalOffers(doc *goquery.Document, baseURL string) []RentalOffer {
	var offers []RentalOffer

	// Check if we have any listings
	listingCount := doc.Find(".list-item-container").Length()
	if listingCount == 0 {
		log.Println("Warning: No rental listings found in the HTML document")
		// Check if there's an error message or empty results message
		errorMsg := doc.Find(".error-message, .no-results-message").Text()
		if errorMsg != "" {
			log.Printf("Message from page: %s", strings.TrimSpace(errorMsg))
		}
	}

	doc.Find(".list-item-container").Each(func(i int, s *goquery.Selection) {
		offer := extractSingleOffer(s, baseURL)

		// If we have enough information, add the offer to our list
		if offer.Size != "" || offer.Rooms != "" || offer.Price != "" {
			offers = append(offers, offer)
		} else {
			log.Printf("Warning: Skipping offer #%d due to insufficient data", i+1)
		}
	})

	return offers
}

// extractSingleOffer extracts a single rental offer from a selection
func extractSingleOffer(s *goquery.Selection, baseURL string) RentalOffer {
	offer := RentalOffer{}

	// Extract address and title from image
	extractAddressAndTitle(s, &offer)

	// Extract price
	extractPrice(s, &offer)

	// Extract size and room information
	extractSizeAndRooms(s, &offer)

	// Extract availability
	extractAvailability(s, &offer)

	// Extract link and fallback address
	extractLinkAndFallbackAddress(s, &offer, baseURL)

	return offer
}

// extractAddressAndTitle extracts address and title from the image
func extractAddressAndTitle(s *goquery.Selection, offer *RentalOffer) {
	// Find the main property image in the listing
	imgEl := s.Find(".col-1 img")
	if imgEl.Length() > 0 {
		// Get the first image that's not an icon (icons typically have small dimensions or specific classes)
		imgEl.Each(func(i int, img *goquery.Selection) {
			if alt, exists := img.Attr("alt"); exists && alt != "" {
				// Skip images that are clearly icons (usually have very short alt text)
				if len(alt) > 5 && !strings.Contains(strings.ToLower(alt), "icon") {
					offer.Address = alt
					// Use the first part of the address as the title (street address)
					parts := strings.Split(alt, ",")
					if len(parts) > 0 {
						offer.Title = strings.TrimSpace(parts[0])
					}
				}
			}
		})
	}
}

// extractPrice extracts the price from the selection
func extractPrice(s *goquery.Selection, offer *RentalOffer) {
	priceEl := s.Find("span.price")
	if priceEl.Length() > 0 {
		offer.Price = strings.TrimSpace(priceEl.Text())
	}
}

// extractSizeAndRooms extracts size and room information from the selection
func extractSizeAndRooms(s *goquery.Selection, offer *RentalOffer) {
	col2El := s.Find(".col-2 .list-unstyled")
	if col2El.Length() > 0 {
		// First li typically contains housing type and size (e.g., "kerrostalo, 34 m²")
		sizeText := strings.TrimSpace(col2El.Find("li").First().Text())
		if strings.Contains(sizeText, "m²") {
			parts := strings.Split(sizeText, ",")
			if len(parts) > 1 {
				offer.Size = strings.TrimSpace(parts[1])
			}
		}

		// Second li typically contains room information (e.g., "1h + alk + kt + ransk.parveke")
		if col2El.Find("li").Length() > 1 {
			roomsText := strings.TrimSpace(col2El.Find("li").Eq(1).Text())
			offer.Rooms = roomsText
		}
	}
}

// extractAvailability extracts availability information from the selection
func extractAvailability(s *goquery.Selection, offer *RentalOffer) {
	availEl := s.Find(".showing-lease-container li")
	if availEl.Length() > 0 {
		offer.Available = strings.TrimSpace(availEl.Text())
	}
}

// extractLinkAndFallbackAddress extracts the link and fallback address from the selection
func extractLinkAndFallbackAddress(s *goquery.Selection, offer *RentalOffer, baseURL string) {
	linkEl := s.Find("a.list-item-link")
	if href, exists := linkEl.Attr("href"); exists {
		if !strings.HasPrefix(href, "http") {
			href = baseURL + href
		}
		offer.Link = href

		// If we don't have an address, try to extract location from the link
		if offer.Address == "" {
			extractAddressFromLink(offer, href)
		}
	}
}

// extractAddressFromLink extracts address information from the link
func extractAddressFromLink(offer *RentalOffer, href string) {
	// Parse the URL path to extract location information
	parsedURL, err := url.Parse(href)
	if err == nil {
		pathParts := strings.Split(strings.Trim(parsedURL.Path, "/"), "/")

		// The URL structure typically follows a pattern like:
		// /vuokra-asunto/[city]/[district]/[type]/[id]
		if len(pathParts) >= 4 {
			// Extract city and district from URL path
			cityIndex := 1     // Typically the second element in the path
			districtIndex := 2 // Typically the third element in the path

			if cityIndex < len(pathParts) && districtIndex < len(pathParts) {
				city := strings.Title(pathParts[cityIndex])
				district := strings.Title(pathParts[districtIndex])

				if offer.Title == "" {
					offer.Title = district
				}
				offer.Address = district + ", " + city
			}
		}
	}
}
