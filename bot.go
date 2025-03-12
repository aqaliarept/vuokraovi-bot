package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aqaliarept/vuokraovi-bot/state"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// BotConfig holds the configuration for the Telegram bot
type BotConfig struct {
	Token          string
	UpdateInterval time.Duration
	DataDir        string
	FormDataFile   string
	MaxPages       int
}

// RunBot starts the bot and runs it indefinitely
func RunBot(config BotConfig) error {
	// Initialize bot
	bot, err := tgbotapi.NewBotAPI(config.Token)
	if err != nil {
		return fmt.Errorf("failed to create bot: %w", err)
	}

	log.Printf("Authorized on account %s", bot.Self.UserName)

	// Initialize bot state
	botState := state.NewBotState(config.DataDir)
	if err := botState.LoadState(); err != nil {
		log.Printf("Warning: Failed to load bot state: %v", err)
	}

	// Set up updates channel
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	// Start periodic update goroutine
	go periodicUpdate(bot, botState, config)

	// Process updates
	for update := range updates {
		if update.Message != nil {
			handleMessage(bot, botState, update.Message, config)
		}
	}

	return nil
}

// periodicUpdate periodically checks for new rental offers and notifies users
func periodicUpdate(bot *tgbotapi.BotAPI, botState *state.BotState, config BotConfig) {
	// Start with a small delay to allow bot to initialize
	time.Sleep(5 * time.Second)

	// Create a ticker for periodic updates
	ticker := time.NewTicker(config.UpdateInterval)
	defer ticker.Stop()

	// Create a channel for the initial update
	initialUpdateDone := make(chan struct{})

	// Start initial update in a separate goroutine
	go func() {
		if err := updateAndNotify(bot, botState, config); err != nil {
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
	for range ticker.C {
		if err := updateAndNotify(bot, botState, config); err != nil {
			log.Printf("Error during periodic update: %v", err)
			continue
		}
	}
}

// updateAndNotify updates the rental offers and notifies users about new offers
func updateAndNotify(bot *tgbotapi.BotAPI, botState *state.BotState, config BotConfig) error {
	log.Println("Checking for new rental offers...")

	// Fetch rental offers
	offers, err := fetchRentalOffers(config.FormDataFile, config.MaxPages)
	if err != nil {
		return fmt.Errorf("error fetching rental offers: %v", err)
	}

	// Update offers in state and get new ones
	newOffers := botState.UpdateOffers(offers)
	if len(newOffers) > 0 {
		log.Printf("Found %d new rental offers", len(newOffers))
		notifyUsers(bot, botState, newOffers)
	} else {
		log.Println("No new rental offers found")
	}

	return nil
}

// fetchRentalOffers fetches rental offers using the WebSite struct
func fetchRentalOffers(formDataFile string, maxPages int) ([]state.RentalOffer, error) {
	// Create website client
	website, err := NewWebSite(false) // verbose=false for bot mode
	if err != nil {
		return nil, fmt.Errorf("error creating website client: %w", err)
	}

	// Read form data from file
	formData, err := os.ReadFile(formDataFile)
	if err != nil {
		return nil, fmt.Errorf("error reading form data from %s: %w", formDataFile, err)
	}

	// Fetch offers using the website client
	offers, err := website.FetchRentalOffers(string(formData), maxPages)
	if err != nil {
		return nil, fmt.Errorf("error fetching rental offers: %w", err)
	}

	// Convert RentalOffer to state.RentalOffer
	stateOffers := make([]state.RentalOffer, len(offers))
	for i, offer := range offers {
		stateOffers[i] = state.RentalOffer{
			Title:     offer.Title,
			Address:   offer.Address,
			Price:     offer.Price,
			Size:      offer.Size,
			Rooms:     offer.Rooms,
			Available: offer.Available,
			Link:      offer.Link,
		}
	}

	return stateOffers, nil
}

// notifyUsers notifies users about new rental offers
func notifyUsers(bot *tgbotapi.BotAPI, botState *state.BotState, newOffers []state.RentalOffer) {
	users := botState.GetAllUsers()

	for chatID := range users {
		if !botState.GetUserNotificationsEnabled(chatID) {
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
			botState.MarkOfferAsSeen(chatID, offer.Link)
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
			botState.UpdateUserLastNotified(chatID, time.Now())
		}
	}
}

// handleMessage handles incoming messages
func handleMessage(bot *tgbotapi.BotAPI, botState *state.BotState, message *tgbotapi.Message, config BotConfig) {
	// Add or update user
	botState.AddUser(message.From, message.Chat.ID)

	// Handle commands and button presses
	switch message.Text {
	case "/start":
		handleStartCommand(bot, botState, message, config)
	case "List Offers 📋", "/list":
		handleListCommand(bot, botState, message)
	case "Reset 🔄", "/reset":
		handleResetCommand(bot, botState, message)
	case "Notifications 🔔", "/notifications":
		handleNotificationsCommand(bot, botState, message)
	case "Status 📊", "/status":
		handleStatusCommand(bot, botState, message, config)
	case "Help ❓", "/help":
		handleHelpCommand(bot, message)
	case "/clear":
		handleClearCommand(bot, botState, message, config)
	case "Enable Notifications 🔔":
		toggleNotifications(bot, botState, message.Chat.ID, true)
	case "Disable Notifications 🔕":
		toggleNotifications(bot, botState, message.Chat.ID, false)
	case "Back to Main Menu ↩️":
		msg := tgbotapi.NewMessage(message.Chat.ID, "Main menu:")
		msg.ReplyMarkup = createMainKeyboard()
		bot.Send(msg)
	case "Yes, Clear Data ✅":
		handleClearConfirm(bot, botState, message.Chat.ID, config)
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

// createMainKeyboard creates the main keyboard markup
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

// toggleNotifications toggles notifications for a user
func toggleNotifications(bot *tgbotapi.BotAPI, botState *state.BotState, chatID int64, enable bool) {
	botState.SetUserNotifications(chatID, enable)

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

// handleStartCommand handles the /start command
func handleStartCommand(bot *tgbotapi.BotAPI, botState *state.BotState, message *tgbotapi.Message, config BotConfig) {
	chatID := message.Chat.ID

	// Welcome message
	welcomeMsg := fmt.Sprintf("👋 Welcome to the Vuokraovi Rental Bot, %s!\n\n", message.From.FirstName)
	welcomeMsg += "I will notify you about new rental offers from Vuokraovi.com.\n\n"
	welcomeMsg += "Use the buttons below or type commands to interact with me:"

	msg := tgbotapi.NewMessage(chatID, welcomeMsg)
	msg.ReplyMarkup = createMainKeyboard()
	bot.Send(msg)

	// Send all current offers to the new user
	offers := make([]state.RentalOffer, 0)
	for _, offer := range botState.GetKnownOffers() {
		offers = append(offers, offer)
	}

	if len(offers) > 0 {
		infoMsg := fmt.Sprintf("Here are the current %d rental offers:", len(offers))
		bot.Send(tgbotapi.NewMessage(chatID, infoMsg))

		sendOffersList(bot, offers, chatID)
	}
}

// handleListCommand handles the /list command
func handleListCommand(bot *tgbotapi.BotAPI, botState *state.BotState, message *tgbotapi.Message) {
	offers := make([]state.RentalOffer, 0)
	for _, offer := range botState.GetKnownOffers() {
		offers = append(offers, offer)
	}

	if len(offers) == 0 {
		msg := tgbotapi.NewMessage(message.Chat.ID, "No rental offers available at the moment.")
		msg.ReplyMarkup = createMainKeyboard()
		bot.Send(msg)
		return
	}

	infoMsg := fmt.Sprintf("Here are the current %d rental offers:", len(offers))
	bot.Send(tgbotapi.NewMessage(message.Chat.ID, infoMsg))

	sendOffersList(bot, offers, message.Chat.ID)
}

// sendOffersList sends a list of offers to a chat
func sendOffersList(bot *tgbotapi.BotAPI, offers []state.RentalOffer, chatID int64) {
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
func handleResetCommand(bot *tgbotapi.BotAPI, botState *state.BotState, message *tgbotapi.Message) {
	botState.ResetUserState(message.Chat.ID)

	msg := tgbotapi.NewMessage(message.Chat.ID, "✅ Your state has been reset. You will now receive all available offers again.")
	msg.ReplyMarkup = createMainKeyboard()
	bot.Send(msg)

	// Send all current offers to the user
	handleListCommand(bot, botState, message)
}

// handleNotificationsCommand handles the /notifications command
func handleNotificationsCommand(bot *tgbotapi.BotAPI, botState *state.BotState, message *tgbotapi.Message) {
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
func handleStatusCommand(bot *tgbotapi.BotAPI, botState *state.BotState, message *tgbotapi.Message, config BotConfig) {
	chatID := message.Chat.ID

	// Get state information
	totalOffers := len(botState.GetKnownOffers())
	lastUpdate := botState.GetLastUpdated()
	notifications, exists := botState.GetUserNotifications(chatID)

	if !exists {
		// Add user if they don't exist
		botState.AddUser(message.From, chatID)
		notifications, _ = botState.GetUserNotifications(chatID)
	}

	statusText := fmt.Sprintf("Bot Status:\n\n"+
		"• Total offers: %d\n"+
		"• Your notifications: %s\n"+
		"• Last update: %s\n"+
		"• Update interval: %v",
		totalOffers,
		map[bool]string{true: "Enabled ✅", false: "Disabled 🔕"}[notifications],
		lastUpdate.Format("2006-01-02 15:04:05"),
		config.UpdateInterval)

	msg := tgbotapi.NewMessage(chatID, statusText)
	msg.ReplyMarkup = createMainKeyboard()
	msg.ParseMode = "Markdown"
	bot.Send(msg)
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

// handleClearCommand handles the /clear command
func handleClearCommand(bot *tgbotapi.BotAPI, botState *state.BotState, message *tgbotapi.Message, config BotConfig) {
	chatID := message.Chat.ID
	_, exists := botState.GetUser(chatID)
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
func handleClearConfirm(bot *tgbotapi.BotAPI, botState *state.BotState, chatID int64, config BotConfig) {
	botState.ResetUserState(chatID)
	msg := tgbotapi.NewMessage(chatID, "✅ Your data has been cleared successfully.\n\n"+
		"• Seen offers have been reset\n"+
		"• Notifications have been re-enabled\n\n"+
		"You will now receive notifications for all offers again.")
	msg.ReplyMarkup = createMainKeyboard()
	bot.Send(msg)
}
