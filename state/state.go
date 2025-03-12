package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// UserState represents the state of a user
type UserState struct {
	ChatID        int64           `json:"chat_id"`
	Username      string          `json:"username"`
	FirstName     string          `json:"first_name"`
	LastName      string          `json:"last_name"`
	LastNotified  time.Time       `json:"last_notified"`
	SeenOffers    map[string]bool `json:"seen_offers"`
	Notifications bool            `json:"notifications"`
}

// RentalOffer represents a rental property listing
type RentalOffer struct {
	Title     string `json:"title"`
	Address   string `json:"address"`
	Price     string `json:"price"`
	Size      string `json:"size"`
	Rooms     string `json:"rooms"`
	Available string `json:"available"`
	Link      string `json:"link"`
}

// BotState represents the state of the bot
type BotState struct {
	Users       map[int64]*UserState   `json:"users"`
	KnownOffers map[string]RentalOffer `json:"known_offers"`
	LastUpdated time.Time              `json:"last_updated"`
	mutex       sync.Mutex             `json:"-"`
	saveDir     string                 `json:"-"`
}

// NewBotState creates a new bot state
func NewBotState(saveDir string) *BotState {
	state := &BotState{
		Users:       make(map[int64]*UserState),
		KnownOffers: make(map[string]RentalOffer),
		LastUpdated: time.Now(),
		saveDir:     saveDir,
	}
	state.LoadState()
	return state
}

// cleanURL removes query parameters from a URL
func cleanURL(url string) string {
	pos := strings.Index(url, "?")
	if pos == -1 {
		return url
	}
	return url[:pos]
}

// SaveState saves the bot state to disk
func (bs *BotState) saveState() error {
	stateCopy := &BotState{
		Users:       make(map[int64]*UserState, len(bs.Users)),
		KnownOffers: make(map[string]RentalOffer, len(bs.KnownOffers)),
		LastUpdated: bs.LastUpdated,
	}

	// Clean up and validate KnownOffers
	for k, v := range bs.KnownOffers {
		cleanLink := cleanURL(k)
		if cleanLink != "" && v.Link != "" {
			stateCopy.KnownOffers[cleanLink] = v
		}
	}

	// Clean up and validate Users
	for k, v := range bs.Users {
		if v == nil {
			continue
		}
		userCopy := *v
		if userCopy.SeenOffers == nil {
			userCopy.SeenOffers = make(map[string]bool)
		}
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

	if err := os.MkdirAll(bs.saveDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	stateFile := filepath.Join(bs.saveDir, "bot_state.json")
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
func (bs *BotState) LoadState() error {
	stateFile := filepath.Join(bs.saveDir, "bot_state.json")
	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		bs.Users = make(map[int64]*UserState)
		bs.KnownOffers = make(map[string]RentalOffer)
		bs.LastUpdated = time.Now()
		return nil
	}

	data, err := os.ReadFile(stateFile)
	if err != nil {
		return fmt.Errorf("failed to read bot state file: %w", err)
	}

	bs.Users = make(map[int64]*UserState)
	bs.KnownOffers = make(map[string]RentalOffer)
	bs.LastUpdated = time.Now()

	var loadedState BotState
	if err := json.Unmarshal(data, &loadedState); err != nil {
		return fmt.Errorf("failed to unmarshal bot state: %w", err)
	}

	if loadedState.Users == nil {
		loadedState.Users = make(map[int64]*UserState)
	}
	if loadedState.KnownOffers == nil {
		loadedState.KnownOffers = make(map[string]RentalOffer)
	}

	uniqueOffers := make(map[string]RentalOffer)
	for k, v := range loadedState.KnownOffers {
		cleanLink := cleanURL(k)
		if cleanLink != "" && v.Link != "" {
			uniqueOffers[cleanLink] = v
		}
	}
	bs.KnownOffers = uniqueOffers

	for k, v := range loadedState.Users {
		if v == nil {
			continue
		}
		userCopy := *v
		if userCopy.SeenOffers == nil {
			userCopy.SeenOffers = make(map[string]bool)
		}
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

// CleanupInactiveUsers removes users who haven't been active for more than 30 days
func (bs *BotState) CleanupInactiveUsers() error {
	bs.mutex.Lock()
	defer bs.mutex.Unlock()

	now := time.Now()
	inactiveThreshold := now.AddDate(0, 0, -30)

	for chatID, user := range bs.Users {
		if user.LastNotified.Before(inactiveThreshold) {
			delete(bs.Users, chatID)
		}
	}

	return bs.saveState()
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
		bs.Users[chatID].Username = user.UserName
		bs.Users[chatID].FirstName = user.FirstName
		bs.Users[chatID].LastName = user.LastName
	}
	bs.saveState()
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

	for _, offer := range offers {
		cleanLink := cleanURL(offer.Link)
		if cleanLink != "" {
			offerCopy := offer
			offerCopy.Link = cleanLink

			if _, exists := bs.KnownOffers[cleanLink]; !exists {
				newOffers = append(newOffers, offerCopy)
				bs.KnownOffers[cleanLink] = offerCopy
			}
		}
	}

	bs.LastUpdated = time.Now()
	bs.saveState()
	return newOffers
}

// ResetUserState resets a user's state
func (bs *BotState) ResetUserState(chatID int64) {
	bs.mutex.Lock()
	defer bs.mutex.Unlock()

	if user, exists := bs.Users[chatID]; exists {
		user.SeenOffers = make(map[string]bool)
		user.LastNotified = time.Time{}
		bs.saveState()
	}
}

// GetKnownOffers returns a copy of all known offers
func (bs *BotState) GetKnownOffers() map[string]RentalOffer {
	bs.mutex.Lock()
	defer bs.mutex.Unlock()

	offers := make(map[string]RentalOffer, len(bs.KnownOffers))
	for k, v := range bs.KnownOffers {
		offers[k] = v
	}
	return offers
}

// GetLastUpdated returns the last updated timestamp
func (bs *BotState) GetLastUpdated() time.Time {
	bs.mutex.Lock()
	defer bs.mutex.Unlock()
	return bs.LastUpdated
}

// SetUserNotifications sets the notifications state for a user
func (bs *BotState) SetUserNotifications(chatID int64, enabled bool) bool {
	bs.mutex.Lock()
	defer bs.mutex.Unlock()

	if user, exists := bs.Users[chatID]; exists {
		user.Notifications = enabled
		bs.saveState()
		return true
	}
	return false
}

// GetUserNotifications gets the notifications state for a user
func (bs *BotState) GetUserNotifications(chatID int64) (bool, bool) {
	bs.mutex.Lock()
	defer bs.mutex.Unlock()

	if user, exists := bs.Users[chatID]; exists {
		return user.Notifications, true
	}
	return false, false
}

// MarkOfferAsSeen marks an offer as seen by a user
func (bs *BotState) MarkOfferAsSeen(chatID int64, offerLink string) {
	bs.mutex.Lock()
	defer bs.mutex.Unlock()

	if user, exists := bs.Users[chatID]; exists {
		if user.SeenOffers == nil {
			user.SeenOffers = make(map[string]bool)
		}
		user.SeenOffers[cleanURL(offerLink)] = true
	}
	bs.saveState()
}

// UpdateUserLastNotified updates the last notified timestamp for a user
func (bs *BotState) UpdateUserLastNotified(chatID int64, t time.Time) {
	bs.mutex.Lock()
	defer bs.mutex.Unlock()

	if user, exists := bs.Users[chatID]; exists {
		user.LastNotified = t
	}
	bs.saveState()
}

// GetUserNotificationsEnabled returns whether a user has notifications enabled
func (bs *BotState) GetUserNotificationsEnabled(chatID int64) bool {
	bs.mutex.Lock()
	defer bs.mutex.Unlock()

	if user, exists := bs.Users[chatID]; exists {
		return user.Notifications
	}
	return false
}

// GetAllUsers returns a copy of all users
func (bs *BotState) GetAllUsers() map[int64]*UserState {
	bs.mutex.Lock()
	defer bs.mutex.Unlock()

	users := make(map[int64]*UserState, len(bs.Users))
	for k, v := range bs.Users {
		userCopy := *v
		users[k] = &userCopy
	}
	return users
}
