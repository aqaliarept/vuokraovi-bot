package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

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

	// Bot mode flags
	botModePtr := flag.Bool("bot", false, "Run in Telegram bot mode")
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	updateIntervalPtr := flag.Int("interval", 30, "Update interval in minutes (for bot mode)")
	dataDirPtr := flag.String("data", "./data", "Directory to store persistent data (for bot mode)")

	flag.Parse()

	// Check if bot mode is enabled
	if *botModePtr {
		// Create bot config
		config := BotConfig{
			Token:          token,
			UpdateInterval: time.Duration(*updateIntervalPtr) * time.Minute,
			DataDir:        *dataDirPtr,
			FormDataFile:   *formDataFilePtr,
			MaxPages:       *maxPagesPtr,
		}

		// Run bot
		log.Println("Starting Vuokraovi Rental Bot...")
		if err := RunBot(config); err != nil {
			log.Fatalf("Error running bot: %v", err)
		}
		return
	}

	// Console mode (original functionality)
	// Set up logging
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Create website client
	website, err := NewWebSite(*verbosePtr)
	if err != nil {
		log.Fatalf("Error creating website client: %v", err)
	}

	// Read form data from file
	formData, err := os.ReadFile(*formDataFilePtr)
	if err != nil {
		log.Fatalf("Error reading form data from %s: %v", *formDataFilePtr, err)
	}

	// Fetch rental offers
	offers, err := website.FetchRentalOffers(string(formData), *maxPagesPtr)
	if err != nil {
		log.Fatalf("Error fetching rental offers: %v", err)
	}

	// Print results
	printResults(offers)
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
