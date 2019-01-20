package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"
)

// PORT holds the port that the server will be communicating on
const PORT = ":3000"

// INTERAC_URL holds the url of the interac API
const INTERAC_URL = "https://gateway-web.beta.interac.ca/publicapi/api/v2/money-requests/send"

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
	dataParsed := "[{" + strings.Split(string(data), "{")[1] + "]"
	var decoded = []donation{}
	dec := json.NewDecoder(strings.NewReader(dataParsed))
	dec.Decode(&decoded)
	*d = decoded[0]
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
	if len(strings.Split(string(data), "{")) < 2 {
		return
	}

	donationRequest.populateFields(r)
	request.populateFields(donationRequest.Name, donationRequest.Email, donationRequest.Amount)

	jsone, err := (json.MarshalIndent(&request, "", " "))

	if err != nil {
		fmt.Println(err.Error)
	}

	fmt.Println(json.Valid(jsone), string(jsone))

	req, err := http.NewRequest("POST", INTERAC_URL, strings.NewReader(string(jsone)))
	if err != nil {
		fmt.Println(err.Error)
	}
	basicAuth(req)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println(err.Error)
	}

	dec := json.NewDecoder(resp.Body)
	var response postResp
	err = dec.Decode(&response)
	if err != nil {
		fmt.Println(err.Error)
	}

	fmt.Println("sendPaymentRequest() Response:", response)
	references[response.ReferenceNumber] = donationOutput{Name: donationRequest.Name, Amount: donationRequest.Amount, Message: donationRequest.Message}
	refsToCheck = append(refsToCheck, response.ReferenceNumber)

	http.Redirect(w, r, "http://www.google.com", 301)
}

func checkPaymentProcessed(refNum string, index int) {
	urlToUse := INTERAC_URL + "?referenceNumber=" + refNum
	req, err := http.NewRequest("GET", urlToUse, nil)
	if err != nil {
		fmt.Println(err.Error)
	}

	basicAuth(req)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println(err.Error)
	}

	dec := json.NewDecoder(resp.Body)
	var response []sentMoneyResponse
	err = dec.Decode(&response)
	if err != nil {
		fmt.Println(err.Error)
	}

	fmt.Println("checkPaymentProcessed() Response:", response[0])

	if response[0].Status == 8 || response[0].Status == 3 {
		fmt.Println("Request Completed")
		delete(references, refNum)
		refsToCheck = append(refsToCheck[:index], refsToCheck[index+1:]...)
	} else if response[0].Status == 4 || response[0].Status == 5 || response[0].Status == 6 || response[0].Status == 7 {
		fmt.Println("Request Failed")
		delete(references, refNum)
		refsToCheck = append(refsToCheck[:index], refsToCheck[index+1:]...)
	}
}

func iteratePayments() {
	var counter int64
	prevTime := time.Now().UTC().UnixNano()
	for true {
		counter += time.Now().UTC().UnixNano() - prevTime
		prevTime = time.Now().UTC().UnixNano()
		if counter > 10000000000 {
			fmt.Println(counter)
			counter = 0
			fmt.Println("Checking Responses")
			for i, ref := range refsToCheck {
				go checkPaymentProcessed(ref, i)
			}
		}
	}
}

func main() {
	references = make(map[string]donationOutput)
	http.HandleFunc("/request", sendPaymentRequest)
	fmt.Println("Starting server on port" + PORT)
	go iteratePayments()
	http.ListenAndServe(PORT, nil)
}
