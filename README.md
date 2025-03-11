# Vuokraovi Rental Bot

A Go application that scrapes rental listings from [Vuokraovi.com](https://www.vuokraovi.com/) and can run as a console application or a Telegram bot.

## Features

### Console Mode

- Sends a POST request with form data to search for rental properties
- Parses the HTML response to extract rental listings
- Follows pagination links to retrieve all listings
- Automatically maintains cookies between requests
- Displays results in a colorful, easy-to-read format
- Supports command-line parameters to control the scraping process

### Telegram Bot Mode

- Periodically checks for new rental offers
- Persists data on disk (users, known offers, etc.)
- Notifies users about new rental offers
- Provides interactive buttons for commands
- Allows users to control notifications and view rental listings

## Prerequisites

- Go 1.21 or higher
- Internet connection
- Telegram Bot Token (for bot mode)

## Installation

1. Clone this repository:

   ```
   git clone https://github.com/aqaliarept/vuokraovi-bot.git
   cd vuokraovi-bot
   ```

2. Install dependencies:
   ```
   go mod download
   ```

## Usage

### Console Mode

```
go run main.go parser.go [options]
```

Available options:

- `-limit N`: Limit the number of pages to query (default: 0 = no limit)
- `-verbose`: Enable verbose logging
- `-form path/to/file`: Specify a custom path to the form data file (default: form_data.txt)

Examples:

```
# Query only the first 2 pages
go run main.go parser.go -limit 2

# Enable verbose logging
go run main.go parser.go -verbose

# Use a custom form data file
go run main.go parser.go -form custom_form_data.txt
```

### Telegram Bot Mode

```
go run main.go bot.go parser.go -bot -token YOUR_TELEGRAM_BOT_TOKEN [options]
```

Additional options:

- `-interval N`: Update interval in minutes (default: 30)
- `-data path/to/dir`: Directory to store persistent data (default: ./data)

Examples:

```
# Run bot with default settings
go run main.go bot.go parser.go -bot -token YOUR_TELEGRAM_BOT_TOKEN

# Run bot with custom update interval and data directory
go run main.go bot.go parser.go -bot -token YOUR_TELEGRAM_BOT_TOKEN -interval 15 -data /path/to/data
```

## Bot Commands

- `/start` - Start the bot and get current offers
- `/help` - Show help message
- `/list` - List all current rental offers
- `/reset` - Reset your state and get all offers again
- `/notifications` - Toggle notifications on/off
- `/status` - Show bot status information

The bot also provides interactive buttons for all commands.

## How It Works

### Console Mode

1. The program sends a POST request to `https://www.vuokraovi.com/haku/vuokra-asunnot?locale=fi` with the form data from `form_data.txt`.
2. It parses the HTML response using the functions in `parser.go` to extract rental listings.
3. The HTTP client automatically maintains cookies between requests using a cookie jar.
4. If the page contains a "next" link, the program follows it to retrieve more listings.
5. The program continues following pagination links until there are no more pages or until the specified limit is reached.
6. All results are printed to the console in a colorful, easy-to-read format.

### Bot Mode

1. The bot periodically checks for new rental offers using the same scraping process as the console mode.
2. When new offers are found, the bot notifies all users who have enabled notifications.
3. Users can interact with the bot using commands or buttons to view listings, toggle notifications, etc.
4. The bot persists its state to disk, so it can be restarted without losing data.

## Files

- `main.go`: The main program file
- `parser.go`: Contains functions for parsing HTML and extracting rental listings
- `bot.go`: Contains the Telegram bot functionality
- `form_data.txt`: Contains the form data for the search request
- `data/`: Directory for persistent data (created automatically in bot mode)

## License

This project is licensed under the MIT License.
