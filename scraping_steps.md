<!-- 1. use this request to load content of the html page

curl -v -L -X POST \
 -H "Content-Type: application/x-www-form-urlencoded" \
 --data-binary @form_data.txt \
 "https://www.vuokraovi.com/haku/vuokra-asunnot?locale=fi" \
 -c cookies.txt

2. this page should be parsed with code from parser.go file

3. if the page contains link like:

<link rel="next" href='https://www.vuokraovi.com/vuokra-asunnot?page=2' />

then request next page with GET with cookies fron the POST request
pages should be requested until the end

then print results to the console -->

NEXT STEPS:

1. add the app mode work as a telegram bot
2. bot should periodically query rental offers
3. persist data on disk
4. new bot user gets the whole list of rentals
5. then bot notifiy user with new rental offers
6. each user state also is persisted on disk and loaded from disk in the case of bot restart
7. bot should have a command of eset current user state and return all available offers
