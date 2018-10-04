package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"
)

var wg sync.WaitGroup
var updateMainMapChannel chan []Property
var updateWithGardenChannel chan []Property
var propertyNumberMap map[string]int
var propertyWithGardenMap map[string]int
var tokenChannel chan bool
var stopChannel chan bool
var mapMutex sync.Mutex

const propertiesURL = "http://partnerapi.funda.nl/feeds/Aanbod.svc/json/ac1b0b1572524640a0ecc54de453ea9f/?type=koop&zo=/amsterdam/&page=%d"
const propertiesWithGardenURL = "http://partnerapi.funda.nl/feeds/Aanbod.svc/json/ac1b0b1572524640a0ecc54de453ea9f/?type=koop&zo=/amsterdam/tuin/&page=%d"

//OuterData property data nested within
type OuterData struct {
	AccountStatus     int        `json:"AccountStatus"`
	EmailNotConfirmed bool       `json:"EmailNotConfirmed"`
	ValidationFailed  bool       `json:"ValidationFailed"`
	PropertyObjects   []Property `json:"Objects"`
	PagingObjs        PagingInfo `json:"Paging"`
}

//PagingInfo need num pages for Get calls
type PagingInfo struct {
	NumberPages int `json:"AantalPaginas"`
}

//Property just need agent name here
type Property struct {
	AgentName string `json:"MakelaarNaam"`
	IsSold    bool   `json:"IsVerkocht"`
}

//PropertyPair as need to sort using slice
type PropertyPair struct {
	AgentName     string
	NumProperties int
}

//TemplateData data passed to html template
type TemplateData struct {
	PropertiesRegular    []PropertyPair
	PropertiesWithGarden []PropertyPair
}

//1. get initial page count. check no pages required to get all data for each api call.
//2. create for loop number of pages. and execute client calls to api
//on seperate go routines.
//3.  desrialize data. append to total make agent name. and append num properties.
// in map form.
//4. backoff for 20 seconds per goroutine if api limit of 100 per minute reached.
//   back off reapeat infinitely if api per goroutine. could implement a number of retries
//   but if the call fails we will be returning incorrect information.
//5. continue. create final map of data and sort highest to lowest.
//6. repeat steps 1 -5 twice for different endpoints.
//7. display data in some for html? or cmd printout.

func main() {

	propertyNumberMap = make(map[string]int)
	propertyWithGardenMap = make(map[string]int)
	updateMainMapChannel = make(chan []Property)
	updateWithGardenChannel = make(chan []Property)
	tokenChannel = make(chan bool, 25)
	stopChannel = make(chan bool)

	pagesRegProperties := getPagingInfo(propertiesURL)
	pagesPropertyWithGarden := getPagingInfo(propertiesWithGardenURL)

	go spinner(100*time.Millisecond, stopChannel)
	wg.Add(2)

	go func(propNumMap map[string]int, pages int) {
		initCall(propertiesURL, pages, propNumMap, updateMainMapChannel)
	}(propertyNumberMap, pagesRegProperties)

	go func(propGardenMap map[string]int, pages int) {
		initCall(propertiesWithGardenURL, pages, propGardenMap, updateWithGardenChannel)
	}(propertyWithGardenMap, pagesPropertyWithGarden)

	wg.Wait()
	stopChannel <- true
	close(tokenChannel)
	close(stopChannel)
	runServer()
}

/**
* initial call to api. get num pages.
*
**/

func getPagingInfo(urlEndpoint string) int {

	url := fmt.Sprintf(urlEndpoint, 1)
	resp, err := http.Get(url)

	if err != nil {
		log.Println(err)
		return -1
	}

	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var outerData OuterData
		json.NewDecoder(resp.Body).Decode(&outerData)
		return outerData.PagingObjs.NumberPages
	}

	return -1
}

func initCall(endPoint string, numPages int, resultsMap map[string]int, propertyDataChannel chan []Property) {

	for i := 1; i <= numPages; i++ {
		wg.Add(1)

		go func(ind int) {
			getPropertyListings(&wg, endPoint, ind, propertyDataChannel)
		}(i)

		go func(mapMutex *sync.Mutex, resultsMap map[string]int) {
			unpdatePropertyMap(&wg, mapMutex, resultsMap, propertyDataChannel)
		}(&mapMutex, resultsMap)
	}

	wg.Done()
}

/**
* execute a bunch of calls to the api extract a map.
* key = name of property manager. value num = of properties
**/

func getPropertyListings(wg *sync.WaitGroup, urlEndpoint string, pageNo int,
	writeChannel chan<- []Property) {

	url := fmt.Sprintf(urlEndpoint, pageNo)

	tokenChannel <- true
	time.Sleep(2 * time.Second)
	resp, err := http.Get(url)
	<-tokenChannel

	if err != nil {
		log.Println(err)
		return
	}

	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var outer OuterData
		json.NewDecoder(resp.Body).Decode(&outer)
		writeChannel <- outer.PropertyObjects

	} else {
		runBackOff()
		getPropertyListings(wg, urlEndpoint, pageNo, writeChannel)
	}
}

func unpdatePropertyMap(wg *sync.WaitGroup, mapMutex *sync.Mutex, propertymap map[string]int,
	readChannel <-chan []Property) {

	for properties := range readChannel {

		mapMutex.Lock()
		for _, property := range properties {

			if _, ok := propertymap[property.AgentName]; ok {
				propertymap[property.AgentName]++
			} else {
				propertymap[property.AgentName] = 1
			}
		}
		mapMutex.Unlock()
		wg.Done()
	}
}

/**
* sleep goroutine for 20 seconds
* used with api call retry
**/

func runBackOff() {
	time.Sleep(20 * time.Second)
}

/**
* sort by largest number properties
* return 10 items in a slice of type PropertyPair
**/

func sortAgents(propertyNumberMap map[string]int) []PropertyPair {

	var propertyPairs []PropertyPair

	for key, val := range propertyNumberMap {
		propertyPairs = append(propertyPairs, PropertyPair{AgentName: key, NumProperties: val})
	}

	sort.Slice(propertyPairs, func(i int, j int) bool {
		return (propertyPairs[i].NumProperties > propertyPairs[j].NumProperties)
	})

	return propertyPairs[:10]
}

func spinner(delay time.Duration, stopChannel chan bool) {

Exit:
	for {
		select {
		default:
			for _, r := range `-\||/` {
				fmt.Printf("\r%c", r)
				time.Sleep(delay)
			}

		case <-stopChannel:
			continue Exit
		}
	}
}

func runServer() {

	fmt.Println("")
	fmt.Println("Listening on port 8080 .....")
	http.HandleFunc("/", handleWebCall)
	http.ListenAndServe(":8080", nil)
}

func handleWebCall(respWriter http.ResponseWriter, req *http.Request) {

	tmpl := template.Must(template.ParseFiles("index.gohtml"))
	data := &TemplateData{
		PropertiesRegular:    sortAgents(propertyNumberMap),
		PropertiesWithGarden: sortAgents(propertyWithGardenMap),
	}
	tmpl.Execute(respWriter, data)
}
