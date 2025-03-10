1. use this request to load content of the html page

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

then print results to the console
