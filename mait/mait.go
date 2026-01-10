package mait

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
)

// URLs for Masdar Alkon (IP: 217.182.195.194)
const (
	BaseURL      = "http://217.182.195.194"
	LoginURL     = BaseURL + "/ints/login"
	SigninURL    = BaseURL + "/ints/signin"
	ReportsPage  = BaseURL + "/ints/agent/SMSCDRReports"
	SMSApiURL    = BaseURL + "/ints/agent/res/data_smscdr.php"
	NumberApiURL = BaseURL + "/ints/agent/res/data_smsnumbers.php"
)

// Response Wrapper
type ApiResponse struct {
	SEcho                interface{}     `json:"sEcho"`
	ITotalRecords        interface{}     `json:"iTotalRecords"`
	ITotalDisplayRecords interface{}     `json:"iTotalDisplayRecords"`
	AAData               [][]interface{} `json:"aaData"`
}

type Client struct {
	HTTPClient *http.Client
	Csstr      string // Only CSSTR is needed now
	Mutex      sync.Mutex
}

// =========================================================
// GLOBAL RAM STORAGE (Specific to 'mait' package only)
// =========================================================
var (
	activeClient *Client    // یہ ویری ایبل صرف اس پینل کا سیشن ہولڈ کرے گا
	clientMutex  sync.Mutex // تھریڈ سیفٹی کے لیے
)

// GetSession: یہ فنکشن ہر بار وہی پرانا کلائنٹ واپس کرے گا (RAM سے)
func GetSession() *Client {
	clientMutex.Lock()
	defer clientMutex.Unlock()

	// اگر کلائنٹ پہلے سے موجود ہے تو وہی واپس کرو
	if activeClient != nil {
		return activeClient
	}

	// اگر پہلی بار کال ہو رہا ہے تو نیا بناؤ اور محفوظ کر لو
	jar, _ := cookiejar.New(nil)
	activeClient = &Client{
		HTTPClient: &http.Client{
			Jar:     jar,
			Timeout: 60 * time.Second,
		},
	}
	return activeClient
}

// ensureSession: Check if we have the Csstr token
func (c *Client) ensureSession() error {
	if c.Csstr != "" {
		return nil
	}
	fmt.Println("[Masdar] Csstr token missing, Login start...")
	return c.performLogin()
}

// performLogin: Hardcoded Credentials as requested
func (c *Client) performLogin() error {
	fmt.Println("[Masdar] >> Step 1: Login Page")
	
	req, _ := http.NewRequest("GET", LoginURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyString := string(bodyBytes)

	// Captcha Logic: What is 5 + 2 = ?
	fmt.Println("[Masdar] >> Step 2: Solving Captcha")
	re := regexp.MustCompile(`What is (\d+) \+ (\d+) = \?`)
	matches := re.FindStringSubmatch(bodyString)
	if len(matches) < 3 {
		return errors.New("captcha math failed")
	}
	num1, _ := strconv.Atoi(matches[1])
	num2, _ := strconv.Atoi(matches[2])
	captchaAns := strconv.Itoa(num1 + num2)
	fmt.Printf("[Masdar] Captcha Solved: %s\n", captchaAns)

	// Step 3: Login POST (HARDCODED CREDENTIALS KEPT)
	data := url.Values{}
	data.Set("username", "Kami526") // Hardcoded User
	data.Set("password", "Kami526") // Hardcoded Pass
	data.Set("capt", captchaAns)

	loginReq, _ := http.NewRequest("POST", SigninURL, bytes.NewBufferString(data.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginReq.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")
	loginReq.Header.Set("Referer", LoginURL)

	resp, err = c.HTTPClient.Do(loginReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Step 4: Get Only Csstr
	fmt.Println("[Masdar] >> Step 3: Getting Csstr Token")
	reportReq, _ := http.NewRequest("GET", ReportsPage, nil)
	reportReq.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")

	resp, err = c.HTTPClient.Do(reportReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	reportBody, _ := io.ReadAll(resp.Body)
	reportString := string(reportBody)
	
	// Regex specifically for 'csstr'
	csstrRe := regexp.MustCompile(`csstr=([^"&']+)`)
	csstrMatch := csstrRe.FindStringSubmatch(reportString)
	
	if len(csstrMatch) > 1 {
		c.Csstr = csstrMatch[1] // SAVED TO RAM OBJECT
		fmt.Println("[Masdar] SUCCESS: Found Csstr:", c.Csstr)
	} else {
		// Fallback regex
		fallbackRe := regexp.MustCompile(`["']csstr["']\s*[:=]\s*["']([^"']+)["']`)
		match2 := fallbackRe.FindStringSubmatch(reportString)
		if len(match2) > 1 {
			c.Csstr = match2[1]
			fmt.Println("[Masdar] SUCCESS: Found Csstr (Fallback):", c.Csstr)
		} else {
			fmt.Println("[Masdar] Warning: Csstr not found! API calls might fail.")
		}
	}

	return nil
}

// ---------------------- SMS CLEANING (Date Logic Updated) ----------------------

func (c *Client) GetSMSLogs() ([]byte, error) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	for i := 0; i < 2; i++ {
		if err := c.ensureSession(); err != nil {
			return nil, err
		}

		// --- NEW DATE LOGIC (Yesterday to Tomorrow) ---
		now := time.Now()
		yesterday := now.AddDate(0, 0, -1)
		tomorrow := now.AddDate(0, 0, 1)

		fdate1 := yesterday.Format("2006-01-02") + " 00:00:00"
		fdate2 := tomorrow.Format("2006-01-02") + " 23:59:59"

		params := url.Values{}
		params.Set("fdate1", fdate1)
		params.Set("fdate2", fdate2)
		params.Set("frange", "")
		params.Set("fclient", "")
		params.Set("fg", "0")
		
		if c.Csstr != "" {
			params.Set("csstr", c.Csstr)
		}

		params.Set("sEcho", "3")
		params.Set("iDisplayLength", "100") 
		params.Set("iSortingCols", "1")
		params.Set("sSortDir_0", "desc")

		finalURL := SMSApiURL + "?" + params.Encode()

		req, _ := http.NewRequest("GET", finalURL, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		// Check Session Expiry
		if bytes.Contains(body, []byte("<!DOCTYPE html>")) {
			fmt.Println("[Masdar] HTML detected (Session Expired), Retrying...")
			c.Csstr = "" // Reset Token so loop retries login
			c.HTTPClient.Jar, _ = cookiejar.New(nil)
			continue
		}

		cleanedJSON, err := cleanMasdarSMS(body)
		if err != nil {
			return nil, err
		}
		return cleanedJSON, nil
	}
	return nil, errors.New("failed after retry")
}

func cleanMasdarSMS(rawJSON []byte) ([]byte, error) {
	var apiResp ApiResponse
	if err := json.Unmarshal(rawJSON, &apiResp); err != nil {
		return rawJSON, nil
	}

	var cleanedRows [][]interface{}

	// Raw: [Date(0), Country(1), Number(2), Service(3), User(4), Message(5), Currency(6), Cost(7), Status(8)]
	// Target: [Date, Country, Number, Service, Message, Currency, Cost, Status] (User Removed)

	for _, row := range apiResp.AAData {
		if len(row) > 8 {
			// Message Cleanup (Index 5 in RAW)
			msg, _ := row[5].(string)
			msg = html.UnescapeString(msg)
			msg = strings.ReplaceAll(msg, "null", "")

			newRow := []interface{}{
				row[0], // Date
				row[1], // Country
				row[2], // Number
				row[3], // Service
				// SKIPPING ROW[4] (User)
				msg,    // Message (Moved to position 4)
				row[6], // Currency
				row[7], // Cost
				row[8], // Status
			}
			cleanedRows = append(cleanedRows, newRow)
		}
	}
	
	apiResp.AAData = cleanedRows
	return json.Marshal(apiResp)
}

// ---------------------- NUMBERS CLEANING (Params Updated) ----------------------

func (c *Client) GetNumberStats() ([]byte, error) {
	c.Mutex.Lock()
	defer c.Mutex.Unlock()

	for i := 0; i < 2; i++ {
		if err := c.ensureSession(); err != nil {
			return nil, err
		}

		// --- UPDATED PARAMS FROM RAW REQUEST ---
		params := url.Values{}
		params.Set("frange", "")
		params.Set("fclient", "")
		
		if c.Csstr != "" {
			params.Set("csstr", c.Csstr)
		}

		// Exact Browser Params to fetch FULL list
		params.Set("sEcho", "2")
		params.Set("iColumns", "8")
		params.Set("sColumns", ",,,,,,,")
		params.Set("iDisplayStart", "0")
		params.Set("iDisplayLength", "-1") // -1 means ALL Records
		
		// Column Maps
		for j := 0; j < 8; j++ {
			idx := strconv.Itoa(j)
			params.Set("mDataProp_"+idx, idx)
			params.Set("sSearch_"+idx, "")
			params.Set("bRegex_"+idx, "false")
			params.Set("bSearchable_"+idx, "true")
			
			// Some cols are sortable, some not in your raw request, 
			// usually setting all to true doesn't hurt, but let's stick to standard
			params.Set("bSortable_"+idx, "true")
		}
		// Col 7 was false in raw request, just safe default
		params.Set("bSortable_7", "false")

		params.Set("sSearch", "")
		params.Set("bRegex", "false")
		params.Set("iSortingCols", "1")
		params.Set("iSortCol_0", "0")
		params.Set("sSortDir_0", "asc")

		finalURL := NumberApiURL + "?" + params.Encode()

		req, _ := http.NewRequest("GET", finalURL, nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K)")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.Header.Set("Referer", BaseURL+"/ints/agent/MySMSNumbers")

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		if bytes.Contains(body, []byte("<!DOCTYPE html>")) {
			fmt.Println("[Masdar] Session Expired on Numbers, Retrying...")
			c.Csstr = ""
			c.HTTPClient.Jar, _ = cookiejar.New(nil)
			continue
		}

		cleanedJSON, err := cleanMasdarNumbers(body)
		if err != nil {
			return nil, err
		}
		return cleanedJSON, nil
	}
	return nil, errors.New("failed after retry")
}

func cleanMasdarNumbers(rawJSON []byte) ([]byte, error) {
	var apiResp ApiResponse
	if err := json.Unmarshal(rawJSON, &apiResp); err != nil {
		return rawJSON, nil
	}

	var cleanedRows [][]interface{}
	rePrice := regexp.MustCompile(`[\d\.]+`)

	for _, row := range apiResp.AAData {
		if len(row) > 7 {
			rangeName := row[1]
			prefix := row[2]
			number := row[3]
			
			priceHTML, _ := row[4].(string)
			billingType := "Weekly"
			if strings.Contains(strings.ToLower(priceHTML), "monthly") {
				billingType = "Monthly"
			}

			currency := "$"
			if strings.Contains(priceHTML, "€") {
				currency = "€"
			} else if strings.Contains(priceHTML, "£") {
				currency = "£"
			}
			
			priceVal := "0"
			matches := rePrice.FindAllString(priceHTML, -1)
			if len(matches) > 0 {
				priceVal = matches[len(matches)-1]
			}
			priceStr := currency + " " + priceVal

			stats := row[7]

			// Target Format: [Country, Prefix, Number, Type, Price, Stats]
			newRow := []interface{}{
				rangeName,
				prefix,
				number,
				billingType,
				priceStr,
				stats,
			}
			cleanedRows = append(cleanedRows, newRow)
		}
	}

	apiResp.AAData = cleanedRows
	apiResp.ITotalRecords = len(cleanedRows)
	apiResp.ITotalDisplayRecords = len(cleanedRows)

	return json.Marshal(apiResp)
}