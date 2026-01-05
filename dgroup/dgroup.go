package dgroup

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

	"github.com/nyaruka/phonenumbers"
)

// URLs (Updated to Client Panel)
const (
	BaseURL      = "http://139.99.63.204"
	LoginURL     = BaseURL + "/ints/login"
	SigninURL    = BaseURL + "/ints/signin"
	ReportsPage  = BaseURL + "/ints/client/SMSCDRStats" // Updated Page
	SMSApiURL    = BaseURL + "/ints/client/res/data_smscdr.php" // Updated Path
	NumberApiURL = BaseURL + "/ints/client/res/data_smsnumbers.php" // Updated Path
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
// GLOBAL RAM STORAGE
// =========================================================
var (
	activeClient *Client
	clientMutex  sync.Mutex
)

func GetSession() *Client {
	clientMutex.Lock()
	defer clientMutex.Unlock()
	if activeClient != nil { return activeClient }
	jar, _ := cookiejar.New(nil)
	activeClient = &Client{
		HTTPClient: &http.Client{Jar: jar, Timeout: 60 * time.Second},
	}
	return activeClient
}

// ---------------------------------------------------------
// LOGIN LOGIC (Updated User/Pass from Capture)
// ---------------------------------------------------------

func (c *Client) ensureSession() error {
	if c.SessKey != "" { return nil }
	return c.performLogin()
}

func (c *Client) performLogin() error {
	fmt.Println("[D-Group Client] >> Step 1: Login Page")
	req, _ := http.NewRequest("GET", LoginURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")

	resp, err := c.HTTPClient.Do(req)
	if err != nil { return err }
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyString := string(bodyBytes)

	// Captcha
	re := regexp.MustCompile(`What is (\d+) \+ (\d+) = \?`)
	matches := re.FindStringSubmatch(bodyString)
	if len(matches) < 3 { return errors.New("captcha math failed") }
	n1, _ := strconv.Atoi(matches[1])
	n2, _ := strconv.Atoi(matches[2])
	captchaAns := strconv.Itoa(n1 + n2)

	// Login Post
	data := url.Values{}
	data.Set("username", "Kami527") // Updated User
	data.Set("password", "Kami526") // Updated Pass
	data.Set("capt", captchaAns)

	loginReq, _ := http.NewRequest("POST", SigninURL, bytes.NewBufferString(data.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginReq.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")
	resp, err = c.HTTPClient.Do(loginReq)
	if err != nil { return err }
	defer resp.Body.Close()

	// Get SessKey (From SMSCDRStats page as per capture)
	reportReq, _ := http.NewRequest("GET", ReportsPage, nil)
	reportReq.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")
	resp, err = c.HTTPClient.Do(reportReq)
	if err != nil { return err }
	defer resp.Body.Close()
	rBody, _ := io.ReadAll(resp.Body)
	
	// SessKey might not be explicit in client panel HTML sometimes, 
	// but let's try to find it or rely on cookies.
	// In your capture, sesskey was NOT in the GET parameters for data_smscdr.php, 
	// but it WAS in previous versions. We will try to find it, but proceed if cookie is enough.
	sessRe := regexp.MustCompile(`sesskey=([a-zA-Z0-9%=]+)`)
	sessMatch := sessRe.FindStringSubmatch(string(rBody))
	
	if len(sessMatch) > 1 {
		c.SessKey = sessMatch[1]
		fmt.Println("[D-Group Client] Found SessKey:", c.SessKey)
	} else {
		fmt.Println("[D-Group Client] SessKey not found in HTML, proceeding with Cookies only.")
		c.SessKey = "dummy" // Mark as logged in
	}

	return nil
}

// ---------------------- SMS LOGIC (Client Panel Params) ----------------------

func (c *Client) GetSMSLogs() ([]byte, error) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	for i := 0; i < 2; i++ {
		if err := c.ensureSession(); err != nil { return nil, err }

		// Date: Yesterday to Tomorrow (Safe window)
		now := time.Now()
		yesterday := now.AddDate(0, 0, -1)
		tomorrow := now.AddDate(0, 0, 1)
		fdate1 := yesterday.Format("2006-01-02") + " 00:00:00"
		fdate2 := tomorrow.Format("2006-01-02") + " 23:59:59"

		params := url.Values{}
		params.Set("fdate1", fdate1)
		params.Set("fdate2", fdate2)
		// Extra params from capture
		params.Set("frange", "")
		params.Set("fnum", "")
		params.Set("fcli", "")
		params.Set("fgdate", "")
		params.Set("fgmonth", "")
		params.Set("fgrange", "")
		params.Set("fgnumber", "")
		params.Set("fgcli", "")
		params.Set("fg", "0")
		
		// DataTable Params (iColumns=7 from capture)
		params.Set("sEcho", "1")
		params.Set("iColumns", "7")
		params.Set("sColumns", ",,,,,,")
		params.Set("iDisplayStart", "0")
		params.Set("iDisplayLength", "100")
		params.Set("sSearch", "")
		params.Set("bRegex", "false")
		params.Set("iSortingCols", "1")
		params.Set("iSortCol_0", "0")
		params.Set("sSortDir_0", "desc") // Sort by Date Descending

		// Column Props
		for j := 0; j < 7; j++ {
			idx := strconv.Itoa(j)
			params.Set("mDataProp_"+idx, idx)
			params.Set("sSearch_"+idx, "")
			params.Set("bRegex_"+idx, "false")
			params.Set("bSearchable_"+idx, "true")
			params.Set("bSortable_"+idx, "true")
		}

		req, _ := http.NewRequest("GET", SMSApiURL+"?"+params.Encode(), nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.Header.Set("Referer", ReportsPage)

		resp, err := c.HTTPClient.Do(req)
		if err != nil { return nil, err }
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		if bytes.Contains(body, []byte("<!DOCTYPE html>")) { // Session expired check
			fmt.Println("[D-Group Client] Session Expired. Re-logging...")
			c.SessKey = ""
			c.HTTPClient.Jar, _ = cookiejar.New(nil)
			continue
		}

		return cleanDGroupSMS(body)
	}
	return nil, errors.New("failed after retry")
}

func cleanDGroupSMS(rawJSON []byte) ([]byte, error) {
	var apiResp ApiResponse
	if err := json.Unmarshal(rawJSON, &apiResp); err != nil { return rawJSON, nil }
	var cleanedRows [][]interface{}

	// Client Panel SMS Response usually:
	// [Date(0), Range(1), Number(2), CLI(3), SMS/Msg(4), Currency(5), Payout(6)]
	// Note: Capture shows columns: Date, Range, Number, CLI, SMS, Currency, My Payout
	
	for _, row := range apiResp.AAData {
		if len(row) > 4 {
			msg, _ := row[4].(string) // SMS Content is likely at index 4 now
			msg = html.UnescapeString(msg)
			msg = strings.ReplaceAll(msg, "null", "")

			newRow := []interface{}{
				row[0], // Date
				row[1], // Range
				row[2], // Number
				row[3], // CLI
				msg,    // SMS
				row[5], // Currency
				row[6], // Payout
				"?",    // Status (Might be missing in client view, or merged)
			}
			cleanedRows = append(cleanedRows, newRow)
		}
	}
	apiResp.AAData = cleanedRows
	return json.Marshal(apiResp)
}

// ---------------------- NUMBERS LOGIC (Client Panel Params) ----------------------

func (c *Client) GetNumberStats() ([]byte, error) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	for i := 0; i < 2; i++ {
		if err := c.ensureSession(); err != nil { return nil, err }

		// Params from Capture (iColumns=6)
		params := url.Values{}
		params.Set("frange", "")
		params.Set("fclient", "") // Capture has this empty
		
		params.Set("sEcho", "2")
		params.Set("iColumns", "6")
		params.Set("sColumns", ",,,,,")
		params.Set("iDisplayStart", "0")
		params.Set("iDisplayLength", "-1") // Fetch ALL
		
		// Column Props (0-5)
		for j := 0; j < 6; j++ {
			idx := strconv.Itoa(j)
			params.Set("mDataProp_"+idx, idx)
			params.Set("sSearch_"+idx, "")
			params.Set("bRegex_"+idx, "false")
			params.Set("bSearchable_"+idx, "true")
			params.Set("bSortable_"+idx, "true")
		}

		params.Set("sSearch", "")
		params.Set("bRegex", "false")
		params.Set("iSortingCols", "1")
		params.Set("iSortCol_0", "0")
		params.Set("sSortDir_0", "asc")

		req, _ := http.NewRequest("GET", NumberApiURL+"?"+params.Encode(), nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.Header.Set("Referer", BaseURL+"/ints/client/MySMSNumbers")

		resp, err := c.HTTPClient.Do(req)
		if err != nil { return nil, err }
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		if bytes.Contains(body, []byte("<!DOCTYPE html>")) {
			fmt.Println("[D-Group Client] Session Expired (Numbers). Re-logging...")
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
	if err := json.Unmarshal(rawJSON, &apiResp); err != nil { return rawJSON, nil }
	var processedRows [][]interface{}

	for _, row := range apiResp.AAData {
		// Client Panel Numbers: [Range(0), Prefix(1), Number(2), Payterm(3), Payout(4), Limits(5)]
		// Note: No 'Status' column in capture HTML table header
		if len(row) > 2 {
			fullNumStr, ok := row[2].(string)
			if !ok { continue }

			countryName := "Unknown"
			countryPrefix := ""
			parseNumStr := fullNumStr
			if !strings.HasPrefix(parseNumStr, "+") { parseNumStr = "+" + parseNumStr }

			numObj, err := phonenumbers.Parse(parseNumStr, "")
			if err == nil {
				countryPrefix = strconv.Itoa(int(numObj.GetCountryCode()))
				regionCode := phonenumbers.GetRegionCodeForNumber(numObj)
				countryName = getCountryName(regionCode)
			}

			newRow := []interface{}{
				countryName,   // 0
				countryPrefix, // 1
				fullNumStr,    // 2
				row[3],        // 3: Payterm
				row[4],        // 4: Payout
				row[5],        // 5: Limits
				"Active",      // 6: Status (Dummy, since column is missing in client view)
			}
			processedRows = append(processedRows, newRow)
		}
	}
	apiResp.AAData = processedRows
	apiResp.ITotalRecords = len(processedRows)
	apiResp.ITotalDisplayRecords = len(processedRows)
	return json.Marshal(apiResp)
}

// Helper to map Region Codes (ISO 2 char) to Full Names
func getCountryName(code string) string {
	code = strings.ToUpper(code)
	countries := map[string]string{
		"AF": "Afghanistan", "AL": "Albania", "DZ": "Algeria", "AO": "Angola", "AR": "Argentina",
		"AM": "Armenia", "AU": "Australia", "AT": "Austria", "AZ": "Azerbaijan", "BH": "Bahrain",
		"BD": "Bangladesh", "BY": "Belarus", "BE": "Belgium", "BO": "Bolivia", "BA": "Bosnia",
		"BR": "Brazil", "BG": "Bulgaria", "KH": "Cambodia", "CM": "Cameroon", "CA": "Canada",
		"CL": "Chile", "CN": "China", "CO": "Colombia", "HR": "Croatia", "CY": "Cyprus",
		"CZ": "Czech Republic", "DK": "Denmark", "EG": "Egypt", "EE": "Estonia", "ET": "Ethiopia",
		"FI": "Finland", "FR": "France", "GE": "Georgia", "DE": "Germany", "GH": "Ghana",
		"GR": "Greece", "HK": "Hong Kong", "HU": "Hungary", "IN": "India", "ID": "Indonesia",
		"IR": "Iran", "IQ": "Iraq", "IE": "Ireland", "IL": "Israel", "IT": "Italy",
		"CI": "Ivory Coast", "JM": "Jamaica", "JP": "Japan", "JO": "Jordan", "KZ": "Kazakhstan",
		"KE": "Kenya", "KW": "Kuwait", "KG": "Kyrgyzstan", "LA": "Laos", "LV": "Latvia",
		"LB": "Lebanon", "LT": "Lithuania", "MY": "Malaysia", "MX": "Mexico", "MD": "Moldova",
		"MN": "Mongolia", "MA": "Morocco", "MM": "Myanmar", "NP": "Nepal", "NL": "Netherlands",
		"NZ": "New Zealand", "NG": "Nigeria", "MK": "North Macedonia", "NO": "Norway", "OM": "Oman",
		"PK": "Pakistan", "PS": "Palestine", "PA": "Panama", "PY": "Paraguay", "PE": "Peru",
		"PH": "Philippines", "PL": "Poland", "PT": "Portugal", "QA": "Qatar", "RO": "Romania",
		"RU": "Russia", "SA": "Saudi Arabia", "RS": "Serbia", "SG": "Singapore", "SK": "Slovakia",
		"SI": "Slovenia", "ZA": "South Africa", "KR": "South Korea", "ES": "Spain", "LK": "Sri Lanka",
		"SE": "Sweden", "CH": "Switzerland", "TW": "Taiwan", "TJ": "Tajikistan", "TZ": "Tanzania",
		"TH": "Thailand", "TN": "Tunisia", "TR": "Turkey", "TM": "Turkmenistan", "UA": "Ukraine",
		"AE": "UAE", "GB": "United Kingdom", "US": "USA", "UY": "Uruguay", "UZ": "Uzbekistan",
		"VE": "Venezuela", "VN": "Vietnam", "YE": "Yemen", "ZM": "Zambia", "ZW": "Zimbabwe",
	}
	if name, ok := countries[code]; ok {
		return name
	}
	return code
}