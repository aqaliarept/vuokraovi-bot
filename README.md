# Vuokraovi Scraper

This is a console application that scrapes rental listings from [Vuokraovi.com](https://www.vuokraovi.com/), a Finnish rental property website.

## Features

- Sends a POST request with form data to search for rental properties
- Parses the HTML response to extract rental listings
- Follows pagination links to retrieve all listings
- Automatically maintains cookies between requests using an HTTP cookie jar
- Displays results in a colorful, easy-to-read format

## Prerequisites

- Go 1.21 or higher
- Internet connection

## Installation

1. Clone this repository:

   ```
   git clone git@github.com:aqaliarept/vuokraovi-bot.git
   cd vuokraovi-bot
   ```

2. Install dependencies:
   ```
   go mod download
   ```

## Usage

1. Make sure you have a `form_data.txt` file with the search parameters. The file should contain URL-encoded form data for the search request.

2. Run the program:

   ```
   go run main.go parser.go
   ```

3. The program will:
   - Send a POST request with the form data
   - Parse the HTML response to extract rental listings
   - Follow pagination links until the end
   - Print the results to the console

## How It Works

1. The program sends a POST request to `https://www.vuokraovi.com/haku/vuokra-asunnot?locale=fi` with the form data from `form_data.txt`.
2. It parses the HTML response using the functions in `parser.go` to extract rental listings.
3. The HTTP client automatically maintains cookies between requests using a cookie jar.
4. If the page contains a "next" link, the program follows it to retrieve more listings.
5. The program continues following pagination links until there are no more pages.
6. All results are printed to the console in a colorful, easy-to-read format.

## Files

- `main.go`: The main program file
- `parser.go`: Contains functions for parsing HTML and extracting rental listings
- `form_data.txt`: Contains the form data for the search request

## License

This project is licensed under the MIT License - see the LICENSE file for details.
