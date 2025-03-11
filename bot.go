package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// BotConfig holds the configuration for the Telegram bot
type BotConfig struct {
	Token          string        // Telegram Bot API token
	UpdateInterval time.Duration // How often to check for new rental offers
	DataDir        string        // Directory to store persistent data
	FormDataFile   string        // Path to form data file
	MaxPages       int           // Maximum number of pages to query
}

// UserState represents the state of a user
type UserState struct {
	ChatID        int64           `json:"chat_id"`
	Username      string          `json:"username"`
	FirstName     string          `json:"first_name"`
	LastName      string          `json:"last_name"`
	LastNotified  time.Time       `json:"last_notified"`
	SeenOffers    map[string]bool `json:"seen_offers"`   // Map of offer links that the user has seen
	Notifications bool            `json:"notifications"` // Whether the user wants to receive notifications
}

// BotState represents the state of the bot
type BotState struct {
	Users       map[int64]*UserState   `json:"users"`
	KnownOffers map[string]RentalOffer `json:"known_offers"` // Map of all known offers by link
	LastUpdated time.Time              `json:"last_updated"`
	mutex       sync.Mutex             // Mutex to protect concurrent access to the state
}

// NewBotState creates a new bot state
func NewBotState() *BotState {
	return &BotState{
		Users:       make(map[int64]*UserState),
		KnownOffers: make(map[string]RentalOffer),
		LastUpdated: time.Now(),
	}
}

// cleanURL removes query parameters from a URL
func cleanURL(url string) string {
	// Find the position of '?' in the URL
	pos := strings.Index(url, "?")
	if pos == -1 {
		return url
	}
	// Return the URL without query parameters
	return url[:pos]
}

// SaveState saves the bot state to disk
func (bs *BotState) SaveState(dataDir string) error {
	// Create a copy of the state to avoid holding the lock during file operations
	bs.mutex.Lock()
	stateCopy := &BotState{
		Users:       make(map[int64]*UserState, len(bs.Users)),
		KnownOffers: make(map[string]RentalOffer, len(bs.KnownOffers)),
		LastUpdated: bs.LastUpdated,
	}

	// First, clean up and validate KnownOffers
	for k, v := range bs.KnownOffers {
		cleanLink := cleanURL(k)
		if cleanLink != "" && v.Link != "" {
			stateCopy.KnownOffers[cleanLink] = v
		}
	}

	// Then, clean up and validate Users
	for k, v := range bs.Users {
		if v == nil {
			continue
		}
		userCopy := *v
		if userCopy.SeenOffers == nil {
			userCopy.SeenOffers = make(map[string]bool)
		}
		// Clean up any invalid seen offers
		validSeenOffers := make(map[string]bool)
		for link := range userCopy.SeenOffers {
			cleanLink := cleanURL(link)
			if _, exists := stateCopy.KnownOffers[cleanLink]; exists {
				validSeenOffers[cleanLink] = true
			}
		}
		userCopy.SeenOffers = validSeenOffers
		stateCopy.Users[k] = &userCopy
	}
	bs.mutex.Unlock()

	// Create data directory if it doesn't exist
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Save bot state
	stateFile := filepath.Join(dataDir, "bot_state.json")
	data, err := json.MarshalIndent(stateCopy, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal bot state: %w", err)
	}

	if err := os.WriteFile(stateFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write bot state file: %w", err)
	}

	return nil
}

// LoadState loads the bot state from disk
func (bs *BotState) LoadState(dataDir string) error {
	bs.mutex.Lock()
	defer bs.mutex.Unlock()

	stateFile := filepath.Join(dataDir, "bot_state.json")
	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		// State file doesn't exist, initialize with empty state
		bs.Users = make(map[int64]*UserState)
		bs.KnownOffers = make(map[string]RentalOffer)
		bs.LastUpdated = time.Now()
		return nil
	}

	data, err := os.ReadFile(stateFile)
	if err != nil {
		return fmt.Errorf("failed to read bot state file: %w", err)
	}

	// Initialize empty state first
	bs.Users = make(map[int64]*UserState)
	bs.KnownOffers = make(map[string]RentalOffer)
	bs.LastUpdated = time.Now()

	var loadedState BotState
	if err := json.Unmarshal(data, &loadedState); err != nil {
		return fmt.Errorf("failed to unmarshal bot state: %w", err)
	}

	// Clean up and validate loaded state
	if loadedState.Users == nil {
		loadedState.Users = make(map[int64]*UserState)
	}
	if loadedState.KnownOffers == nil {
		loadedState.KnownOffers = make(map[string]RentalOffer)
	}

	// First, clean up and validate KnownOffers
	uniqueOffers := make(map[string]RentalOffer)
	for k, v := range loadedState.KnownOffers {
		cleanLink := cleanURL(k)
		if cleanLink != "" && v.Link != "" {
			uniqueOffers[cleanLink] = v
		}
	}
	bs.KnownOffers = uniqueOffers

	// Then, clean up and validate Users
	for k, v := range loadedState.Users {
		if v == nil {
			continue
		}
		userCopy := *v
		if userCopy.SeenOffers == nil {
			userCopy.SeenOffers = make(map[string]bool)
		}
		// Clean up any invalid seen offers
		validSeenOffers := make(map[string]bool)
		for link := range userCopy.SeenOffers {
			cleanLink := cleanURL(link)
			if _, exists := bs.KnownOffers[cleanLink]; exists {
				validSeenOffers[cleanLink] = true
			}
		}
		userCopy.SeenOffers = validSeenOffers
		bs.Users[k] = &userCopy
	}

	if !loadedState.LastUpdated.IsZero() {
		bs.LastUpdated = loadedState.LastUpdated
	}

	return nil
}

// cleanupState ensures data consistency by removing any duplicates
func (bs *BotState) cleanupState() {
	// Clean up duplicate offers
	uniqueOffers := make(map[string]RentalOffer, len(bs.KnownOffers))
	for _, offer := range bs.KnownOffers {
		cleanLink := cleanURL(offer.Link)
		uniqueOffers[cleanLink] = offer
	}
	bs.KnownOffers = uniqueOffers

	// Clean up duplicate seen offers for each user
	for _, user := range bs.Users {
		if user.SeenOffers == nil {
			user.SeenOffers = make(map[string]bool)
		}
		uniqueSeen := make(map[string]bool, len(user.SeenOffers))
		for link := range user.SeenOffers {
			cleanLink := cleanURL(link)
			uniqueSeen[cleanLink] = true
		}
		user.SeenOffers = uniqueSeen
	}
}

// CleanupInactiveUsers removes users who haven't been active for more than 30 days
func (bs *BotState) CleanupInactiveUsers(dataDir string) error {
	bs.mutex.Lock()
	defer bs.mutex.Unlock()

	now := time.Now()
	inactiveThreshold := now.AddDate(0, 0, -30) // 30 days ago

	for chatID, user := range bs.Users {
		if user.LastNotified.Before(inactiveThreshold) {
			delete(bs.Users, chatID)
		}
	}

	return bs.SaveState(dataDir)
}

// AddUser adds a new user to the bot state
func (bs *BotState) AddUser(user *tgbotapi.User, chatID int64) *UserState {
	bs.mutex.Lock()
	defer bs.mutex.Unlock()

	if _, exists := bs.Users[chatID]; !exists {
		bs.Users[chatID] = &UserState{
			ChatID:        chatID,
			Username:      user.UserName,
			FirstName:     user.FirstName,
			LastName:      user.LastName,
			LastNotified:  time.Time{},
			SeenOffers:    make(map[string]bool),
			Notifications: true,
		}
	} else {
		// Update user info if it has changed
		bs.Users[chatID].Username = user.UserName
		bs.Users[chatID].FirstName = user.FirstName
		bs.Users[chatID].LastName = user.LastName
	}

	return bs.Users[chatID]
}

// GetUser gets a user from the bot state
func (bs *BotState) GetUser(chatID int64) (*UserState, bool) {
	bs.mutex.Lock()
	defer bs.mutex.Unlock()

	user, exists := bs.Users[chatID]
	return user, exists
}

// UpdateOffers updates the known offers in the bot state
func (bs *BotState) UpdateOffers(offers []RentalOffer) []RentalOffer {
	bs.mutex.Lock()
	defer bs.mutex.Unlock()

	var newOffers []RentalOffer

	// Check for new offers
	for _, offer := range offers {
		cleanLink := cleanURL(offer.Link)
		if cleanLink != "" {
			// Create a copy of the offer with cleaned link
			offerCopy := offer
			offerCopy.Link = cleanLink

			if _, exists := bs.KnownOffers[cleanLink]; !exists {
				newOffers = append(newOffers, offerCopy)
				bs.KnownOffers[cleanLink] = offerCopy
			}
		}
	}

	bs.LastUpdated = time.Now()
	return newOffers
}

// ResetUserState resets a user's state
func (bs *BotState) ResetUserState(chatID int64) {
	bs.mutex.Lock()
	defer bs.mutex.Unlock()

	if user, exists := bs.Users[chatID]; exists {
		user.SeenOffers = make(map[string]bool)
		user.LastNotified = time.Time{}
	}
}

// Remove setupCommands function as we'll use buttons instead
func RunBot(config BotConfig) error {
	// Initialize bot
	bot, err := tgbotapi.NewBotAPI(config.Token)
	if err != nil {
		return fmt.Errorf("failed to create bot: %w", err)
	}

	log.Printf("Authorized on account %s", bot.Self.UserName)

	// Initialize bot state
	state := NewBotState()
	if err := state.LoadState(config.DataDir); err != nil {
		log.Printf("Warning: Failed to load bot state: %v", err)
	}

	// Set up updates channel
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	// Start periodic update goroutine
	go periodicUpdate(bot, state, config)

	// Process updates
	for update := range updates {
		if update.Message != nil {
			handleMessage(bot, state, update.Message, config)
		}
	}

	return nil
}

// periodicUpdate periodically checks for new rental offers and notifies users
func periodicUpdate(bot *tgbotapi.BotAPI, state *BotState, config BotConfig) {
	// Start with a small delay to allow bot to initialize
	time.Sleep(5 * time.Second)

	// Create a ticker for periodic updates
	ticker := time.NewTicker(config.UpdateInterval)
	defer ticker.Stop()

	// Create a channel for the initial update
	initialUpdateDone := make(chan struct{})

	// Start initial update in a separate goroutine
	go func() {
		if err := updateAndNotify(bot, state, config); err != nil {
			log.Printf("Error during initial update: %v", err)
		}
		close(initialUpdateDone)
	}()

	// Wait for initial update to complete or timeout
	select {
	case <-initialUpdateDone:
		log.Println("Initial update completed successfully")
	case <-time.After(30 * time.Second):
		log.Println("Initial update timed out, continuing with periodic updates")
	}

	// Continue with periodic updates
	for {
		select {
		case <-ticker.C:
			if err := updateAndNotify(bot, state, config); err != nil {
				log.Printf("Error during periodic update: %v", err)
				// Continue with next update even if this one failed
				continue
			}
		}
	}
}

// updateAndNotify updates the rental offers and notifies users about new offers
func updateAndNotify(bot *tgbotapi.BotAPI, state *BotState, config BotConfig) error {
	log.Println("Checking for new rental offers...")

	// Fetch rental offers
	offers, err := fetchRentalOffers(config.FormDataFile, config.MaxPages)
	if err != nil {
		return fmt.Errorf("error fetching rental offers: %w", err)
	}

	// Update offers in state
	newOffers := state.UpdateOffers(offers)
	if len(newOffers) > 0 {
		log.Printf("Found %d new rental offers", len(newOffers))

		// Notify users about new offers
		notifyUsers(bot, state, newOffers)

		// Save state
		if err := state.SaveState(config.DataDir); err != nil {
			log.Printf("Error saving bot state: %v", err)
		}
	} else {
		log.Println("No new rental offers found")
	}

	return nil
}

// fetchRentalOffers fetches rental offers using the existing functionality
func fetchRentalOffers(formDataFile string, maxPages int) ([]RentalOffer, error) {
	// Create a temporary client
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

	// Base URL for the website
	baseURL := "https://www.vuokraovi.com"

	// Read form data from file
	formData, err := os.ReadFile(formDataFile)
	if err != nil {
		return nil, fmt.Errorf("error reading form data from %s: %w", formDataFile, err)
	}

	// Send initial POST request
	initialURL := "https://www.vuokraovi.com/haku/vuokra-asunnot?locale=fi"
	offers, nextPageURL, err := fetchAndParse(client, initialURL, "POST", string(formData), baseURL, false)
	if err != nil {
		return nil, fmt.Errorf("error fetching initial page: %w", err)
	}

	// Use a map to track unique offers by their links
	uniqueOffers := make(map[string]RentalOffer)
	for _, offer := range offers {
		cleanLink := cleanURL(offer.Link)
		if cleanLink != "" {
			// Create a copy of the offer with cleaned link
			offerCopy := offer
			offerCopy.Link = cleanLink
			uniqueOffers[cleanLink] = offerCopy
		}
	}

	// Follow pagination links until the end or until max pages is reached
	pageNum := 2
	for nextPageURL != "" {
		// Check if we've reached the maximum number of pages
		if maxPages > 0 && pageNum > maxPages {
			break
		}

		pageOffers, newNextPageURL, err := fetchAndParse(client, nextPageURL, "GET", "", baseURL, false)
		if err != nil {
			log.Printf("Error fetching page %d: %v", pageNum, err)
			break
		}

		// Add only unique offers from this page
		for _, offer := range pageOffers {
			cleanLink := cleanURL(offer.Link)
			if cleanLink != "" {
				// Create a copy of the offer with cleaned link
				offerCopy := offer
				offerCopy.Link = cleanLink
				if _, exists := uniqueOffers[cleanLink]; !exists {
					uniqueOffers[cleanLink] = offerCopy
				}
			}
		}

		nextPageURL = newNextPageURL
		pageNum++
	}

	// Convert map back to slice
	allOffers := make([]RentalOffer, 0, len(uniqueOffers))
	for _, offer := range uniqueOffers {
		allOffers = append(allOffers, offer)
	}

	return allOffers, nil
}

// notifyUsers notifies users about new rental offers
func notifyUsers(bot *tgbotapi.BotAPI, state *BotState, newOffers []RentalOffer) {
	state.mutex.Lock()
	defer state.mutex.Unlock()

	for chatID, user := range state.Users {
		if !user.Notifications {
			continue
		}

		// Prepare message
		message := fmt.Sprintf("🏠 *New Rental Offers*\n\nFound %d new rental offers:\n\n", len(newOffers))

		// Add offers to message
		for i, offer := range newOffers {
			if i >= 10 {
				message += fmt.Sprintf("\n...and %d more offers. Use /list to see all offers.", len(newOffers)-10)
				break
			}

			message += fmt.Sprintf("*%s*\n", offer.Title)
			message += fmt.Sprintf("📍 %s\n", offer.Address)
			message += fmt.Sprintf("💰 %s\n", offer.Price)
			message += fmt.Sprintf("🛏 %s\n", offer.Rooms)
			message += fmt.Sprintf("📐 %s\n", offer.Size)
			if offer.Available != "" {
				message += fmt.Sprintf("📅 %s\n", offer.Available)
			}
			message += fmt.Sprintf("🔗 [View Details](%s)\n\n", offer.Link)

			// Mark offer as seen by this user
			user.SeenOffers[offer.Link] = true
		}

		// Create keyboard with list button
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("View All Offers 📋", "list_all"),
			),
		)

		// Send message
		msg := tgbotapi.NewMessage(chatID, message)
		msg.ParseMode = "Markdown"
		msg.DisableWebPagePreview = true
		msg.ReplyMarkup = keyboard

		if _, err := bot.Send(msg); err != nil {
			log.Printf("Error sending message to user %d: %v", chatID, err)
		} else {
			user.LastNotified = time.Now()
		}
	}
}

// handleMessage handles incoming messages
func handleMessage(bot *tgbotapi.BotAPI, state *BotState, message *tgbotapi.Message, config BotConfig) {
	// Add or update user
	_ = state.AddUser(message.From, message.Chat.ID)

	// Handle commands and button presses
	switch message.Text {
	case "/start":
		handleStartCommand(bot, state, message, config)
	case "List Offers 📋", "/list":
		handleListCommand(bot, state, message)
	case "Reset 🔄", "/reset":
		handleResetCommand(bot, state, message)
	case "Notifications 🔔", "/notifications":
		handleNotificationsCommand(bot, state, message)
	case "Status 📊", "/status":
		handleStatusCommand(bot, state, message, config)
	case "Help ❓", "/help":
		handleHelpCommand(bot, message)
	case "/clear":
		handleClearCommand(bot, state, message, config)
	case "Enable Notifications 🔔":
		toggleNotifications(bot, state, message.Chat.ID, true)
	case "Disable Notifications 🔕":
		toggleNotifications(bot, state, message.Chat.ID, false)
	case "Back to Main Menu ↩️":
		msg := tgbotapi.NewMessage(message.Chat.ID, "Main menu:")
		msg.ReplyMarkup = createMainKeyboard()
		bot.Send(msg)
	case "Yes, Clear Data ✅":
		handleClearConfirm(bot, state, message.Chat.ID, config)
	case "No, Keep Data ❌":
		msg := tgbotapi.NewMessage(message.Chat.ID, "Data clearing cancelled. Your data is safe.")
		msg.ReplyMarkup = createMainKeyboard()
		bot.Send(msg)
	default:
		msg := tgbotapi.NewMessage(message.Chat.ID, "Please use the buttons below or commands to interact with me:")
		msg.ReplyMarkup = createMainKeyboard()
		bot.Send(msg)
	}
}

// createMainKeyboard to use reply keyboard instead of inline
func createMainKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("List Offers 📋"),
			tgbotapi.NewKeyboardButton("Reset 🔄"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Notifications 🔔"),
			tgbotapi.NewKeyboardButton("Status 📊"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Help ❓"),
		),
	)
}

// handleCallbackQuery handles callback queries from inline keyboards
func handleCallbackQuery(bot *tgbotapi.BotAPI, state *BotState, query *tgbotapi.CallbackQuery, config BotConfig) {
	// Acknowledge the callback query
	callback := tgbotapi.NewCallback(query.ID, "")
	bot.Request(callback)

	// Handle different callback data
	switch query.Data {
	case "notifications_on":
		toggleNotifications(bot, state, query.Message.Chat.ID, true)
	case "notifications_off":
		toggleNotifications(bot, state, query.Message.Chat.ID, false)
	case "list_all":
		handleListCommand(bot, state, &tgbotapi.Message{Chat: query.Message.Chat, From: query.From})
	case "reset":
		handleResetCommand(bot, state, &tgbotapi.Message{Chat: query.Message.Chat, From: query.From})
	case "notifications":
		handleNotificationsCommand(bot, state, &tgbotapi.Message{Chat: query.Message.Chat, From: query.From})
	case "status":
		handleStatusCommand(bot, state, &tgbotapi.Message{Chat: query.Message.Chat, From: query.From}, config)
	case "help":
		handleHelpCommand(bot, &tgbotapi.Message{Chat: query.Message.Chat, From: query.From})
	case "clear_confirm":
		handleClearConfirm(bot, state, query.Message.Chat.ID, config)
	case "clear_cancel":
		msg := tgbotapi.NewMessage(query.Message.Chat.ID, "Data clearing cancelled. Your data is safe.")
		msg.ReplyMarkup = createMainKeyboard()
		bot.Send(msg)
	}
}

// toggleNotifications toggles notifications for a user
func toggleNotifications(bot *tgbotapi.BotAPI, state *BotState, chatID int64, enable bool) {
	state.mutex.Lock()
	defer state.mutex.Unlock()

	if user, exists := state.Users[chatID]; exists {
		user.Notifications = enable

		var message string
		if enable {
			message = "✅ Notifications are now enabled. You will receive updates about new rental offers."
		} else {
			message = "🔕 Notifications are now disabled. You will not receive updates about new rental offers."
		}

		msg := tgbotapi.NewMessage(chatID, message)
		msg.ReplyMarkup = createMainKeyboard()
		bot.Send(msg)
	}
}

// handleStartCommand handles the /start command
func handleStartCommand(bot *tgbotapi.BotAPI, state *BotState, message *tgbotapi.Message, config BotConfig) {
	chatID := message.Chat.ID

	// Welcome message
	welcomeMsg := fmt.Sprintf("👋 Welcome to the Vuokraovi Rental Bot, %s!\n\n", message.From.FirstName)
	welcomeMsg += "I will notify you about new rental offers from Vuokraovi.com.\n\n"
	welcomeMsg += "Use the buttons below or type commands to interact with me:"

	msg := tgbotapi.NewMessage(chatID, welcomeMsg)
	msg.ReplyMarkup = createMainKeyboard()
	bot.Send(msg)

	// Send all current offers to the new user
	state.mutex.Lock()
	offers := make([]RentalOffer, 0, len(state.KnownOffers))
	for _, offer := range state.KnownOffers {
		offers = append(offers, offer)
	}
	state.mutex.Unlock()

	if len(offers) > 0 {
		infoMsg := fmt.Sprintf("Here are the current %d rental offers:", len(offers))
		bot.Send(tgbotapi.NewMessage(chatID, infoMsg))

		sendOffersList(bot, offers, chatID)
	}
}

// handleHelpCommand handles the /help command
func handleHelpCommand(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	helpText := "🤖 *Vuokraovi Rental Bot Commands*\n\n"
	helpText += "/start - Start the bot and get current offers\n"
	helpText += "/help - Show this help message\n"
	helpText += "/list - List all current rental offers\n"
	helpText += "/reset - Reset your state and get all offers again\n"
	helpText += "/notifications - Toggle notifications on/off\n"
	helpText += "/status - Show bot status information\n"
	helpText += "/clear - Clear your data and reset all settings\n\n"
	helpText += "You can also use the buttons below for quick access to commands:"

	msg := tgbotapi.NewMessage(message.Chat.ID, helpText)
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = createMainKeyboard()
	bot.Send(msg)
}

// handleListCommand handles the /list command
func handleListCommand(bot *tgbotapi.BotAPI, state *BotState, message *tgbotapi.Message) {
	state.mutex.Lock()
	offers := make([]RentalOffer, 0, len(state.KnownOffers))
	for _, offer := range state.KnownOffers {
		offers = append(offers, offer)
	}
	state.mutex.Unlock()

	if len(offers) == 0 {
		msg := tgbotapi.NewMessage(message.Chat.ID, "No rental offers available at the moment.")
		msg.ReplyMarkup = createMainKeyboard()
		bot.Send(msg)
		return
	}

	infoMsg := fmt.Sprintf("Here are the current %d rental offers:", len(offers))
	infoMessage := tgbotapi.NewMessage(message.Chat.ID, infoMsg)
	infoMessage.ReplyMarkup = createMainKeyboard()
	bot.Send(infoMessage)

	sendOffersList(bot, offers, message.Chat.ID)
}

// sendOffersList sends a list of offers to a chat
func sendOffersList(bot *tgbotapi.BotAPI, offers []RentalOffer, chatID int64) {
	// Split offers into chunks to avoid message size limits
	chunkSize := 5
	for i := 0; i < len(offers); i += chunkSize {
		end := i + chunkSize
		if end > len(offers) {
			end = len(offers)
		}

		chunk := offers[i:end]
		message := ""

		for _, offer := range chunk {
			message += fmt.Sprintf("*%s*\n", offer.Title)
			message += fmt.Sprintf("📍 %s\n", offer.Address)
			message += fmt.Sprintf("💰 %s\n", offer.Price)
			message += fmt.Sprintf("🛏 %s\n", offer.Rooms)
			message += fmt.Sprintf("📐 %s\n", offer.Size)
			if offer.Available != "" {
				message += fmt.Sprintf("📅 %s\n", offer.Available)
			}
			message += fmt.Sprintf("🔗 [View Details](%s)\n\n", offer.Link)
		}

		// For the last chunk, add the main keyboard
		var markup interface{} = nil
		if end >= len(offers) {
			markup = createMainKeyboard()
		}

		msg := tgbotapi.NewMessage(chatID, message)
		msg.ParseMode = "Markdown"
		msg.DisableWebPagePreview = true
		msg.ReplyMarkup = markup
		bot.Send(msg)

		// Add a small delay to avoid hitting rate limits
		time.Sleep(500 * time.Millisecond)
	}
}

// handleResetCommand handles the /reset command
func handleResetCommand(bot *tgbotapi.BotAPI, state *BotState, message *tgbotapi.Message) {
	state.ResetUserState(message.Chat.ID)

	msg := tgbotapi.NewMessage(message.Chat.ID, "✅ Your state has been reset. You will now receive all available offers again.")
	msg.ReplyMarkup = createMainKeyboard()
	bot.Send(msg)

	// Send all current offers to the user
	handleListCommand(bot, state, message)
}

// handleNotificationsCommand handles the /notifications command
func handleNotificationsCommand(bot *tgbotapi.BotAPI, state *BotState, message *tgbotapi.Message) {
	keyboard := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Enable Notifications 🔔"),
			tgbotapi.NewKeyboardButton("Disable Notifications 🔕"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Back to Main Menu ↩️"),
		),
	)

	msg := tgbotapi.NewMessage(message.Chat.ID, "Do you want to receive notifications about new rental offers?")
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

// handleStatusCommand handles the /status command
func handleStatusCommand(bot *tgbotapi.BotAPI, state *BotState, message *tgbotapi.Message, config BotConfig) {
	chatID := message.Chat.ID

	// Get state information
	state.mutex.Lock()
	totalOffers := len(state.KnownOffers)
	lastUpdate := state.LastUpdated
	user, exists := state.Users[chatID]
	state.mutex.Unlock()

	if !exists {
		// Add user if they don't exist
		state.AddUser(message.From, chatID)
		user = state.Users[chatID]
	}

	// Create main keyboard
	keyboard := createMainKeyboard()

	statusText := fmt.Sprintf("Bot Status:\n\n"+
		"• Total offers: %d\n"+
		"• Your notifications: %s\n"+
		"• Last update: %s",
		totalOffers,
		map[bool]string{true: "Enabled", false: "Disabled"}[user.Notifications],
		lastUpdate.Format("2006-01-02 15:04:05"))

	msg := tgbotapi.NewMessage(chatID, statusText)
	msg.ReplyMarkup = keyboard
	msg.ParseMode = "Markdown"
	bot.Send(msg)
}

// handleClearCommand handles the /clear command
func handleClearCommand(bot *tgbotapi.BotAPI, state *BotState, message *tgbotapi.Message, config BotConfig) {
	chatID := message.Chat.ID
	_, exists := state.GetUser(chatID)
	if !exists {
		msg := tgbotapi.NewMessage(chatID, "Please start the bot first with /start")
		msg.ReplyMarkup = createMainKeyboard()
		bot.Send(msg)
		return
	}

	keyboard := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Yes, Clear Data ✅"),
			tgbotapi.NewKeyboardButton("No, Keep Data ❌"),
		),
	)

	msg := tgbotapi.NewMessage(chatID, "⚠️ Are you sure you want to clear your data? This will:\n\n"+
		"• Remove all your seen offers\n"+
		"• Reset your notification settings\n"+
		"• Clear your last active time\n\n"+
		"This action cannot be undone.")
	msg.ReplyMarkup = keyboard
	bot.Send(msg)
}

// handleClearConfirm handles the confirmation of clearing user data
func handleClearConfirm(bot *tgbotapi.BotAPI, state *BotState, chatID int64, config BotConfig) {
	state.mutex.Lock()
	defer state.mutex.Unlock()

	if user, exists := state.Users[chatID]; exists {
		// Reset user state
		user.SeenOffers = make(map[string]bool)
		user.LastNotified = time.Time{}
		user.Notifications = true

		// Save state
		if err := state.SaveState(config.DataDir); err != nil {
			log.Printf("Error saving state after clear: %v", err)
		}

		msg := tgbotapi.NewMessage(chatID, "✅ Your data has been cleared successfully.\n\n"+
			"• Seen offers have been reset\n"+
			"• Notifications have been re-enabled\n\n"+
			"You will now receive notifications for all offers again.")
		msg.ReplyMarkup = createMainKeyboard()
		bot.Send(msg)
	}
}

// boolToEmoji converts a boolean to an emoji
func boolToEmoji(b bool) string {
	if b {
		return "✅ Enabled"
	}
	return "❌ Disabled"
}
