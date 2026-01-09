package numberpanel1

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	// Google's libphonenumber library for Go
	"github.com/nyaruka/phonenumbers"
)

// URLs for Number Panel (Client Account)
const (
	BaseURL      = "http://51.89.99.105"
	LoginURL     = BaseURL + "/NumberPanel/login"
	SigninURL    = BaseURL + "/NumberPanel/signin"
	ReportsPage  = BaseURL + "/NumberPanel/client/SMSCDRStats" 
	SMSApiURL    = BaseURL + "/NumberPanel/client/res/data_smscdr.php"
	NumberApiURL = BaseURL + "/NumberPanel/client/res/data_smsnumbers.php"
)

// Wrapper for JSON Response
type ApiResponse struct {
	SEcho                interface{}     `json:"sEcho"`
	ITotalRecords        interface{}     `json:"iTotalRecords"`
	ITotalDisplayRecords interface{}     `json:"iTotalDisplayRecords"`
	AAData               [][]interface{} `json:"aaData"`
}

type Client struct {
	HTTPClient *http.Client
	SessKey    string
	Mutex      sync.Mutex
}

// =========================================================
// GLOBAL RAM STORAGE (یہ سیشن کو سنبھال کر رکھے گا)
// =========================================================
var (
	activeClient *Client    
	clientMutex  sync.Mutex
)

// GetSession: Returns existing client or creates new
func GetSession() *Client {
	clientMutex.Lock()
	defer clientMutex.Unlock()

	// اگر کلائنٹ پہلے سے بنا ہوا ہے اور اس میں کی موجود ہے، تو وہی واپس بھیج دے گا (No Login)
	if activeClient != nil {
		return activeClient
	}

	jar, _ := cookiejar.New(nil)
	activeClient = &Client{
		HTTPClient: &http.Client{
			Jar:     jar,
			Timeout: 60 * time.Second,
		},
	}
	return activeClient
}

// ---------------------------------------------------------
// LOGIN LOGIC
// ---------------------------------------------------------

func (c *Client) ensureSession() error {
	// اگر سیشن کی پہلے سے موجود ہے تو لاگ ان نہیں کرے گا
	if c.SessKey != "" {
		return nil
	}
	fmt.Println("[NumberPanel] Session Key missing, Login start...")
	return c.performLogin()
}

func (c *Client) performLogin() error {
	fmt.Println("[NumberPanel] >> Step 1: Login Page (Fetching Captcha)")
	
	req, _ := http.NewRequest("GET", LoginURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Mobile Safari/537.36")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyString := string(bodyBytes)

	// Captcha Logic
	fmt.Println("[NumberPanel] >> Step 2: Solving Captcha")
	re := regexp.MustCompile(`What is (\d+) \+ (\d+) = \?`)
	matches := re.FindStringSubmatch(bodyString)
	if len(matches) < 3 {
		return errors.New("captcha math regex failed")
	}
	num1, _ := strconv.Atoi(matches[1])
	num2, _ := strconv.Atoi(matches[2])
	captchaAns := strconv.Itoa(num1 + num2)
	fmt.Printf("[NumberPanel] Captcha Solved: %s + %s = %s\n", matches[1], matches[2], captchaAns)

	// Step 3: Login POST
	data := url.Values{}
	data.Set("username", "Kami520")   
	data.Set("password", "Kami526")   
	data.Set("capt", captchaAns)

	loginReq, _ := http.NewRequest("POST", SigninURL, bytes.NewBufferString(data.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginReq.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Mobile Safari/537.36")
	loginReq.Header.Set("Referer", LoginURL)
	loginReq.Header.Set("Origin", BaseURL)

	resp, err = c.HTTPClient.Do(loginReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Step 4: Get SessKey
	fmt.Println("[NumberPanel] >> Step 3: Getting SessKey from Dashboard")
	reportReq, _ := http.NewRequest("GET", ReportsPage, nil)
	reportReq.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Mobile Safari/537.36")
	reportReq.Header.Set("Referer", BaseURL+"/NumberPanel/client/SMSDashboard")

	resp, err = c.HTTPClient.Do(reportReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	reportBody, _ := io.ReadAll(resp.Body)
	
	sessRe := regexp.MustCompile(`sesskey=([a-zA-Z0-9%=]+)`)
	sessMatch := sessRe.FindStringSubmatch(string(reportBody))
	
	if len(sessMatch) > 1 {
		c.SessKey = sessMatch[1] // Key saved in RAM
		fmt.Println("[NumberPanel] SessKey Found & Saved:", c.SessKey)
	} else {
		return errors.New("sesskey not found (Login likely failed)")
	}

	return nil
}

// ---------------------- SMS CLEANING (TODAY ONLY) ----------------------

// ---------------------- SMS CLEANING (Matches Node.js Logic) ----------------------

func (c *Client) GetSMSLogs() ([]byte, error) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	for i := 0; i < 2; i++ {
		if err := c.ensureSession(); err != nil {
			if i == 0 {
				c.SessKey = ""
				c.HTTPClient.Jar, _ = cookiejar.New(nil)
				continue
			}
			return nil, err
		}

		// MATCHING NODE.JS DATE LOGIC (Hardcoded Wide Range)
		// Node.js uses: fdate1=2026-01-07 00:00:00 & fdate2=2259-12-20 23:59:59
		// We will stick to dynamic "Today" to keep it useful, OR wide range if you prefer.
		// Let's use specific dates that are known to work in your Node code to test.
		
		// For safety, let's use the Node.js logic:
		fdate1 := "2026-01-07 00:00:00"
		fdate2 := "2259-12-20 23:59:59"

		params := url.Values{}
		params.Set("fdate1", fdate1)
		params.Set("fdate2", fdate2)
		params.Set("frange", "")
		params.Set("fnum", "")
		params.Set("fcli", "")
		params.Set("fgdate", "")
		params.Set("fgmonth", "")
		params.Set("fgrange", "")
		params.Set("fgnumber", "")
		params.Set("fgcli", "")
		params.Set("fg", "0")
		
		// Note: We set sesskey directly. 
		// Go's url.Encode() handles special chars correctly.
		params.Set("sesskey", c.SessKey)
		
		params.Set("sEcho", "2") // Node.js uses 2
		params.Set("iColumns", "7")
		params.Set("sColumns", ",,,,,,")
		params.Set("iDisplayStart", "0")
		params.Set("iDisplayLength", "-1") // Node.js uses -1
		params.Set("mDataProp_0", "0")
		params.Set("sSearch_0", "")
		params.Set("bRegex_0", "false")
		params.Set("bSearchable_0", "true")
		params.Set("bSortable_0", "true")
		params.Set("mDataProp_1", "1")
		params.Set("sSearch_1", "")
		params.Set("bRegex_1", "false")
		params.Set("bSearchable_1", "true")
		params.Set("bSortable_1", "true")
		params.Set("mDataProp_2", "2")
		params.Set("sSearch_2", "")
		params.Set("bRegex_2", "false")
		params.Set("bSearchable_2", "true")
		params.Set("bSortable_2", "true")
		params.Set("mDataProp_3", "3")
		params.Set("sSearch_3", "")
		params.Set("bRegex_3", "false")
		params.Set("bSearchable_3", "true")
		params.Set("bSortable_3", "true")
		params.Set("mDataProp_4", "4")
		params.Set("sSearch_4", "")
		params.Set("bRegex_4", "false")
		params.Set("bSearchable_4", "true")
		params.Set("bSortable_4", "true")
		params.Set("mDataProp_5", "5")
		params.Set("sSearch_5", "")
		params.Set("bRegex_5", "false")
		params.Set("bSearchable_5", "true")
		params.Set("bSortable_5", "true")
		params.Set("mDataProp_6", "6")
		params.Set("sSearch_6", "")
		params.Set("bRegex_6", "false")
		params.Set("bSearchable_6", "true")
		params.Set("bSortable_6", "true")
		params.Set("sSearch", "")
		params.Set("bRegex", "false")
		params.Set("iSortingCols", "1")
		params.Set("iSortCol_0", "0")
		params.Set("sSortDir_0", "desc")

		// Construct URL
		// Note: QueryEscape might turn space into + or %20. 
		// Some PHP servers are picky. Go uses + by default for query params.
		// Let's manually build the query string if needed, but standard Encode() usually works.
		finalURL := SMSApiURL + "?" + params.Encode()

		req, _ := http.NewRequest("GET", finalURL, nil)
		
		// HEADERS FROM NODE.JS
		req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 13; V2040 Build/TP1A.220624.014) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.7499.34 Mobile Safari/537.36")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.Header.Set("Origin", BaseURL) // Added Origin
		req.Header.Set("Referer", ReportsPage)
		req.Header.Set("Accept-Language", "en-US,en;q=0.9,ur-PK;q=0.8,ur;q=0.7") // Added Language

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		// Check for HTML (Session Expiry)
		if bytes.Contains(body, []byte("<!DOCTYPE HTML>")) || bytes.Contains(body, []byte("<html")) || bytes.Contains(body, []byte("login")) {
			fmt.Println("[NumberPanel] Session Expired (HTML received). Re-logging...")
			c.SessKey = ""
			c.HTTPClient.Jar, _ = cookiejar.New(nil)
			continue
		}

		return cleanNumberPanelSMS(body)
	}
	return nil, errors.New("failed after retry")
}

func cleanNumberPanelSMS(rawJSON []byte) ([]byte, error) {
	var apiResp ApiResponse
	if err := json.Unmarshal(rawJSON, &apiResp); err != nil {
		return rawJSON, nil
	}

	var cleanedRows [][]interface{}

	for _, row := range apiResp.AAData {
		// Client Panel Raw Data usually: 
		// [0]Date, [1]Range, [2]Number, [3]Sender, [4]Message, [5]Cost/Status
		
		if len(row) > 4 {
			msg, _ := row[4].(string) // Message is usually at index 4
			msg = html.UnescapeString(msg)
			msg = strings.ReplaceAll(msg, "null", "")

			// Extract Price/Cost if available (usually at index 5)
			cost := "0"
			if len(row) > 5 {
				if val, ok := row[5].(string); ok {
					cost = val
				}
			}

			// =======================================================
			// FINAL D-GROUP STYLE STRUCTURE
			// =======================================================
			newRow := []interface{}{
				row[0], // 0: Date (Time)
				row[1], // 1: Range Name / Country (D-Group style position)
				row[2], // 2: Phone Number
				row[3], // 3: Service Name (Sender ID)
				msg,    // 4: Full Message Content
				"$",    // 5: Currency Symbol (Hardcoded to match D-Group)
				cost,   // 6: Cost/Price
			}
			cleanedRows = append(cleanedRows, newRow)
		}
	}
	apiResp.AAData = cleanedRows
	return json.Marshal(apiResp)
}

// ---------------------- NUMBERS CLEANING (1st Jan to Today) ----------------------

// ---------------------- NUMBERS CLEANING (1st Jan to Today) ----------------------


func (c *Client) GetNumberStats() ([]byte, error) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	for i := 0; i < 2; i++ {
		if err := c.ensureSession(); err != nil {
			if i == 0 {
				c.SessKey = ""
				continue
			}
			return nil, err
		}

		params := url.Values{}
		params.Set("frange", "")
		params.Set("fclient", "")
		params.Set("sEcho", "4")
		params.Set("iColumns", "6")
		params.Set("sColumns", ",,,,,")
		params.Set("iDisplayStart", "0")
		params.Set("iDisplayLength", "-1") // Fetch All
		params.Set("sSearch", "")
		params.Set("bRegex", "false")
		params.Set("iSortCol_0", "0")
		params.Set("sSortDir_0", "asc")
		params.Set("iSortingCols", "1")

		// Map Data Props
		for j := 0; j < 6; j++ {
			idx := strconv.Itoa(j)
			params.Set("mDataProp_"+idx, idx)
			params.Set("sSearch_"+idx, "")
			params.Set("bRegex_"+idx, "false")
			params.Set("bSearchable_"+idx, "true")
			params.Set("bSortable_"+idx, "true")
		}

		finalURL := NumberApiURL + "?" + params.Encode()

		req, _ := http.NewRequest("GET", finalURL, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Mobile Safari/537.36")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.Header.Set("Referer", BaseURL+"/NumberPanel/client/MySMSNumbers")

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		if bytes.Contains(body, []byte("<!DOCTYPE HTML>")) || bytes.Contains(body, []byte("<html")) {
			fmt.Println("[NumberPanel] Session Expired (Numbers). Re-logging...")
			c.SessKey = ""
			c.HTTPClient.Jar, _ = cookiejar.New(nil)
			continue
		}

		return processNumbersWithCountry(body)
	}
	return nil, errors.New("failed after retry")
}

func processNumbersWithCountry(rawJSON []byte) ([]byte, error) {
	var apiResp ApiResponse
	if err := json.Unmarshal(rawJSON, &apiResp); err != nil {
		return rawJSON, nil
	}

	var processedRows [][]interface{}

	for _, row := range apiResp.AAData {
		// Client API Raw Structure:
		// [0] Range Name
		// [1] Empty
		// [2] Full Number
		// [3] Period
		// [4] Price
		// [5] Stats

		if len(row) >= 6 {
			rangeName, _ := row[0].(string) // [0] Range Name
			numberStr, _ := row[2].(string) // [2] Number
			period, _ := row[3].(string)    // [3] Period
			price, _ := row[4].(string)     // [4] Price
			statsRaw, _ := row[5].(string)  // [5] Stats

			// 1. Clean Number (Remove spaces/dashes)
			numberStr = strings.ReplaceAll(numberStr, " ", "")
			numberStr = strings.ReplaceAll(numberStr, "-", "")

			// 2. Get Country Code for Index [1]
			countryCodeStr := "" // Default empty if parse fails
			
			// Add + for parsing if missing
			parseNumStr := numberStr
			if !strings.HasPrefix(parseNumStr, "+") {
				parseNumStr = "+" + parseNumStr
			}

			// Use Library to extract Country Code (e.g. 213, 92)
			numObj, err := phonenumbers.Parse(parseNumStr, "")
			if err == nil {
				countryCodeStr = strconv.Itoa(int(numObj.GetCountryCode()))
			} else {
				// If parsing fails, try to extract first few digits manually or leave empty
				if len(numberStr) > 4 {
					countryCodeStr = numberStr[:3] 
				}
			}

			// 3. Clean Stats HTML
			statsClean := strings.ReplaceAll(statsRaw, "<b>", "")
			statsClean = strings.ReplaceAll(statsClean, "</b>", "")
			statsClean = strings.TrimSpace(statsClean)

			// =======================================================
			// FINAL MAIT-STYLE STRUCTURE
			// =======================================================
			newRow := []interface{}{
				rangeName,      // [0] Main Title (e.g. Algeria-Exclusive...)
				countryCodeStr, // [1] Subtitle 1: Country Code (e.g. 213)
				numberStr,      // [2] Subtitle 2: Full Number
				period,         // [3] Period (Weekly/Monthly)
				price,          // [4] Price
				statsClean,     // [5] Bottom Stats
			}
			processedRows = append(processedRows, newRow)
		}
	}

	apiResp.AAData = processedRows
	apiResp.ITotalRecords = len(processedRows)
	apiResp.ITotalDisplayRecords = len(processedRows)

	return json.Marshal(apiResp)
}

func getCountryName(code string) string {
	code = strings.ToUpper(code)
	countries := map[string]string{
		"AF": "Afghanistan", "PK": "Pakistan", "US": "USA", "GB": "United Kingdom",
		"IN": "India", "BD": "Bangladesh", "CN": "China", "RU": "Russia",
		"CA": "Canada", "AU": "Australia", "DE": "Germany", "FR": "France",
		"TR": "Turkey", "SA": "Saudi Arabia", "AE": "UAE",
	}
	if name, ok := countries[code]; ok { return name }
	return code
}