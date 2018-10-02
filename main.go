package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

var wg sync.WaitGroup
var mapChannel chan map[string]int
var mapPropertyChannel chan []Property
var propertyNumberMap map[string]int
var tokenChannel chan bool
var errorChannel chan error
var mapMutex sync.Mutex

const propertiesURL = "http://partnerapi.funda.nl/feeds/Aanbod.svc/json/ac1b0b1572524640a0ecc54de453ea9f/?type=koop&zo=/amsterdam/&page=%d"

//OuterData property data nested within
type OuterData struct {
	AccountStatus     int        `json:"AccountStatus"`
	EmailNotConfirmed bool       `json:"EmailNotConfirmed"`
	ValidationFailed  bool       `json:"ValidationFailed"`
	PropertyObjects   []Property `json:"Objects"`
}

//Property just need agent name here
type Property struct {
	AgentName string `json:"MakelaarNaam"`
	IsSold    bool   `json:"IsVerkocht"`
}

//1. get initial page. check no pages required to get all data.
//2. create for loop number of pages. and execute client calls to api
//on seperate go routines.
//3.  desrialize data. append to total make agent name. and append num properties.
// in map form.
//4. backoff for 1 minute if api limit of 100 reached.(show some loading dialog etc for this??)
//5. continue. create final map of data and sort highest to lowest.
//6. display data in some for html? or cmd printout.

func main() {

	propertyNumberMap = make(map[string]int)
	mapPropertyChannel = make(chan []Property)
	errorChannel = make(chan error)
	tokenChannel = make(chan bool, 20)
	pageNo := 50

	for i := 1; i <= pageNo; i++ {

		wg.Add(1)

		go func(ind int) {
			getFromWeb(&wg, ind)
		}(i)

		go func(mapMutex *sync.Mutex) {
			unpdateMainPropertyMap(&wg, mapMutex)
		}(&mapMutex)
	}

	wg.Wait()
	fmt.Println("printing map ....")
	fmt.Println(propertyNumberMap)

	close(tokenChannel)
}

/**
* execute a bunch of calls to the api extract a map.
* key = name of property manager. value num = of properties
**/

func getFromWeb(wg *sync.WaitGroup, pageNo int) {

	url := fmt.Sprintf(propertiesURL, pageNo)

	tokenChannel <- true
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
		mapPropertyChannel <- outer.PropertyObjects

	} else {
		fmt.Println(fmt.Sprintf("failed with resp code : %d and page no: %d", resp.StatusCode, pageNo))
		fmt.Println(resp.Status)
		wg.Done()
	}
}

func unpdateMainPropertyMap(wg *sync.WaitGroup, mapMutex *sync.Mutex) {

	for properties := range mapPropertyChannel {

		mapMutex.Lock()
		for _, property := range properties {

			if _, ok := propertyNumberMap[property.AgentName]; ok {
				propertyNumberMap[property.AgentName]++
			} else {
				propertyNumberMap[property.AgentName] = 1
			}
		}
		mapMutex.Unlock()
		wg.Done()
	}

	close(mapPropertyChannel)
}

func showLoadingDialog() {

	go func() {

	}()

}

//sleep main go routine
func runBackOff() {
	time.Sleep(1 * time.Minute)
}

func sortAgents() {

}

func displayTable() {

}
