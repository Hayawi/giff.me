package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"os"
	"strconv"
	"strings"
	"time"
)

// PORT holds the port that the server will be communicating on
const PORT = "3000"

// INTERAC_URL holds the url of the interac API
const INTERAC_URL = "https://gateway-web.beta.interac.ca/publicapi/api/v2/money-requests/send"

const STREAMER_URL = "https://secret-taiga-61915.herokuapp.com/"

var references map[string]donationOutput
var refsToCheck []string

type postResp struct {
	ReferenceNumber   string `json:"referenceNumber"`
	PaymentGatewayURL string `json:"paymentGatewayUrl"`
}

type donationOutput struct {
	Name    string
	Amount  float64
	Message string
}

type sentMoneyResponse struct {
	Status int
}

type donation struct {
	Amount  float64
	Email   string
	Message string
	Name    string
}

type moneyRequest struct {
	SourceMoneyRequestID          string          `json:"sourceMoneyRequestId"`
	RequestedFrom                 json.RawMessage `json:"requestedFrom"`
	Amount                        float64         `json:"amount"`
	Currency                      string          `json:"currency"`
	EditableFulfillAmount         bool            `json:"editableFulfillAmount"`
	RequesterMessage              string          `json:"requesterMessage"`
	ExpiryDate                    string          `json:"expiryDate"`
	SupressResponderNotifications bool            `json:"supressResponderNotifications"`
}

type jsonKV struct {
	key   string
	value string
}

func randomID() string {
	id := ""
	characters := []string{"a", "b", "c", "d", "e", "f", "g", "h",
		"i", "j", "k", "l", "m", "n", "o", "p", "q", "r", "s", "t",
		"u", "v", "w", "x", "y", "z", "0", "1", "2", "3", "4", "5",
		"6", "7", "8", "9", "-"}

	rand.Seed(time.Now().UTC().UnixNano())
	for i := 0; i < 32; i++ {
		id += characters[rand.Int()%len(characters)]
	}

	return id
}

func (d *donation) populateFields(r *http.Request) {
	data, _ := httputil.DumpRequest(r, true)
	formBeginIndex := strings.Index(string(data), "name=")
	//data = []byte(string(data)[0:formBeginIndex] + "{" + string(data)[formBeginIndex:] + "}")

	dataParsed := string(data)[formBeginIndex:]

	splitData := strings.Split(dataParsed, "&")
	m := make(map[string]string)
	for _, pair := range splitData {
		z := strings.Split(pair, "=")
		m[z[0]] = z[1]
	}

	d.Name = m["name"]
	d.Amount, _ = strconv.ParseFloat(m["amount"], 32)
	d.Email = strings.Replace(m["email"], "%40", "@", -1)
	d.Message = m["message"]

	// var decoded = []donation{}
	// dec := json.NewDecoder(strings.NewReader(string(dataJson)))
	// dec.Decode(&decoded)
	// log.Println(decoded)
	// *d = decoded[0]
}

func (m *moneyRequest) populateFields(name string, email string, amount float64) {
	m.Currency = "CAD"
	m.EditableFulfillAmount = false
	m.ExpiryDate = "2019-01-24T04:59:59.899Z"
	m.SupressResponderNotifications = false
	m.RequesterMessage = "Confirm your recent donation! Thanks for using giff.me ❤️ "
	m.RequestedFrom = json.RawMessage(`{"contactName":"` + name + `","language":"en","notificationPreferences":[{"handle":"` + email + `","handleType":"email","active":"true"}]}`)

	m.Amount = amount
	m.SourceMoneyRequestID = randomID()
}

func basicAuth(req *http.Request) {
	id := map[string]string{"accessToken": "Bearer 138a9922-bb12-4c00-86d6-6fa42b868cdf",
		"thirdPartyAccessId": "CA1TAWb2RnjGSWQ2",
		"requestId":          "4453a5b3-fd72-481f-b796-8efbe47b0d22",
		"deviceId ":          "49794efc-aefd-4e4c-a4e8-2d013ade09a9",
		"apiRegistrationId":  "CA1ARRdDwHU5xkzc"}

	req.Header.Add("Accept", "application/json")
	req.Header.Add("accessToken", id["accessToken"])
	req.Header.Add("thirdPartyAccessId", id["thirdPartyAccessId"])
	req.Header.Add("requestId", "4453ae4b-fd92-4sgf-b796-8ef2e33b0d22")
	req.Header.Add("deviceId", id["deviceId"])
	req.Header.Add("apiRegistrationId", id["apiRegistrationId"])
	req.Header.Add("Content-Type", "application/json")
}

func sendPaymentRequest(w http.ResponseWriter, r *http.Request) {
	var donationRequest donation
	var request moneyRequest

	data, _ := httputil.DumpRequest(r, true)
	formBeginIndex := strings.Index(string(data), "name=")
	data = []byte(string(data)[0:formBeginIndex] + "{" + string(data)[formBeginIndex:] + "}")
	if len(strings.Split(string(data), "{")) < 2 {
		return
	}

	donationRequest.populateFields(r)
	request.populateFields(donationRequest.Name, donationRequest.Email, donationRequest.Amount)

	jsone, err := (json.MarshalIndent(&request, "", " "))

	if err != nil {
		log.Println(err.Error)
	}

	log.Println(json.Valid(jsone), string(jsone))

	req, err := http.NewRequest("POST", INTERAC_URL, strings.NewReader(string(jsone)))
	if err != nil {
		log.Println(err.Error)
	}
	basicAuth(req)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println(err.Error)
	}

	dec := json.NewDecoder(resp.Body)
	var response postResp
	err = dec.Decode(&response)
	if err != nil {
		log.Println(err.Error)
	}

	log.Println("sendPaymentRequest() Response:", response)
	references[response.ReferenceNumber] = donationOutput{Name: donationRequest.Name, Amount: donationRequest.Amount, Message: donationRequest.Message}
	refsToCheck = append(refsToCheck, response.ReferenceNumber)

	fmt.Fprintf(w, "Thank you for using giff.me. Please check your email to confirm payment.")
}

func checkPaymentProcessed(refNum string, index int) {
	urlToUse := INTERAC_URL + "?referenceNumber=" + refNum
	req, err := http.NewRequest("GET", urlToUse, nil)
	if err != nil {
		log.Println(err.Error)
	}

	basicAuth(req)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println(err.Error)
	}

	dec := json.NewDecoder(resp.Body)
	var response []sentMoneyResponse
	err = dec.Decode(&response)
	if err != nil {
		log.Println(err.Error)
	}

	log.Println("checkPaymentProcessed() Response:", response[0])

	if response[0].Status == 8 || response[0].Status == 3 {
		log.Println("Request Completed")
		submitDonation(refNum)
		delete(references, refNum)
		refsToCheck = append(refsToCheck[:index], refsToCheck[index+1:]...)
	} else if response[0].Status == 4 || response[0].Status == 5 || response[0].Status == 6 || response[0].Status == 7 {
		log.Println("Request Failed")
		delete(references, refNum)
		refsToCheck = append(refsToCheck[:index], refsToCheck[index+1:]...)
	}
}

func submitDonation(refNum string) {
	name := references[refNum].Name
	message := references[refNum].Message
	amount := references[refNum].Amount
	urlToUse := STREAMER_URL + "?name=" + name + "&message=" + message + "&amount=" + fmt.Sprintf("%.2f", amount)

	http.Get(urlToUse)
}

func iteratePayments() {
	var counter int64
	prevTime := time.Now().UTC().UnixNano()
	for true {
		counter += time.Now().UTC().UnixNano() - prevTime
		prevTime = time.Now().UTC().UnixNano()
		if counter > 500000000 {
			log.Println(counter)
			counter = 0
			log.Println("Checking Responses")
			for i, ref := range refsToCheck {
				go checkPaymentProcessed(ref, i)
			}
		}
	}
}

func main() {
	references = make(map[string]donationOutput)
	var portToUse = os.Getenv("PORT")
	// Set a default port if there is nothing in the environment
	if portToUse == "" {
		portToUse = PORT
		log.Println("INFO: No PORT environment variable detected, defaulting to " + portToUse)
	}
	http.HandleFunc("/request", sendPaymentRequest)
	log.Println("Starting server on port " + portToUse)
	go iteratePayments()
	http.ListenAndServe(":"+portToUse, nil)
}
