package mait

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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

// URLs
const (
	BaseURL      = "http://217.182.195.194"
	LoginURL     = BaseURL + "/ints/login"
	SigninURL    = BaseURL + "/ints/signin"
	ReportsPage  = BaseURL + "/ints/agent/SMSCDRReports"
	SMSApiURL    = BaseURL + "/ints/agent/res/data_smscdr.php"
	NumberApiURL = BaseURL + "/ints/agent/res/data_smsnumbers.php"
)

// Response Struct
type ApiResponse struct {
	SEcho                interface{}     `json:"sEcho"`
	ITotalRecords        interface{}     `json:"iTotalRecords"`
	ITotalDisplayRecords interface{}     `json:"iTotalDisplayRecords"`
	AAData               [][]interface{} `json:"aaData"`
}

type Client struct {
	HTTPClient *http.Client
	Csstr      string 
	Mutex      sync.Mutex // API Call Lock (optional usage)
	LoginMutex sync.Mutex // Login Specific Lock
}

var (
	activeClient *Client
	once         sync.Once
)

func GetSession() *Client {
	once.Do(func() {
		jar, _ := cookiejar.New(nil)
		activeClient = &Client{
			HTTPClient: &http.Client{
				Jar:     jar,
				Timeout: 60 * time.Second,
			},
		}
	})
	return activeClient
}

// ---------------------------------------------------------
// SMART LOGIN LOGIC (Prevent Multiple Logins)
// ---------------------------------------------------------

// ensureSession: صرف تب چلے گا جب ایپ پہلی بار سٹارٹ ہو گی
func (c *Client) ensureSession() error {
	// یہ صرف ریڈ کرتا ہے، اس لیے لاک کی ضرورت نہیں اگر ٹوکن موجود ہو
	if c.Csstr != "" {
		return nil
	}
	// اگر ٹوکن نہیں ہے، تو فورس لاگ ان کرو
	return c.ForceRelogin("")
}

// ForceRelogin: یہ فنکشن ساری ٹریفک روک کر صرف ایک بار لاگ ان کرے گا
func (c *Client) ForceRelogin(failedToken string) error {
	c.LoginMutex.Lock()
	defer c.LoginMutex.Unlock()

	// CRITICAL CHECK:
	// چیک کرو کہ جب ہم لاک کے لیے انتظار کر رہے تھے، کیا کسی اور نے لاگ ان کر دیا؟
	// اگر موجودہ ٹوکن 'failedToken' سے مختلف ہے اور خالی نہیں ہے، تو اس کا مطلب 
	// کسی اور تھریڈ نے سیشن اپڈیٹ کر دیا ہے۔ ہمیں دوبارہ لاگ ان کی ضرورت نہیں ہے۔
	if c.Csstr != "" && c.Csstr != failedToken {
		fmt.Println("[Masdar] Another routine already refreshed the session. Using it.")
		return nil
	}

	fmt.Println("[Masdar] 🛑 Blocking requests. Performing SINGLE Login...")
	
	// تھوڑا سا ڈیلے تاکہ اگر سرور غصے میں ہے تو ٹھنڈا ہو جائے
	time.Sleep(1 * time.Second)

	return c.performLogin()
}

func (c *Client) performLogin() error {
	// Step 1: Login Page
	req, _ := http.NewRequest("GET", LoginURL, nil)
	c.setCommonHeaders(req)
	
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyString := string(bodyBytes)

	// Step 2: Solve Captcha
	re := regexp.MustCompile(`What\s+is\s+(\d+)\s*\+\s*(\d+)`)
	matches := re.FindStringSubmatch(bodyString)
	
	if len(matches) < 3 {
		if strings.Contains(bodyString, "Forbidden") {
			fmt.Println("[Masdar] ⚠️ Still getting 403 Forbidden. IP might be temporarily blocked.")
			return errors.New("SERVER BLOCKED IP (403 Forbidden)")
		}
		return errors.New("captcha regex failed")
	}

	num1, _ := strconv.Atoi(matches[1])
	num2, _ := strconv.Atoi(matches[2])
	captchaAns := strconv.Itoa(num1 + num2)
	fmt.Printf("[Masdar] Captcha Solved: %s + %s = %s\n", matches[1], matches[2], captchaAns)

	// Step 3: Post Login
	data := url.Values{}
	data.Set("username", "Kami526") 
	data.Set("password", "Kami526") 
	data.Set("capt", captchaAns)

	loginReq, _ := http.NewRequest("POST", SigninURL, bytes.NewBufferString(data.Encode()))
	c.setCommonHeaders(loginReq)
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginReq.Header.Set("Referer", LoginURL)
	loginReq.Header.Set("Origin", BaseURL)

	resp, err = c.HTTPClient.Do(loginReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Step 4: Extract Csstr Token
	reportReq, _ := http.NewRequest("GET", ReportsPage, nil)
	c.setCommonHeaders(reportReq)
	reportReq.Header.Set("Referer", BaseURL+"/ints/agent/MySMSNumbers")

	resp, err = c.HTTPClient.Do(reportReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	reportBody, _ := io.ReadAll(resp.Body)
	reportString := string(reportBody)
	
	csstrRe := regexp.MustCompile(`csstr=([a-zA-Z0-9]+)`)
	csstrMatch := csstrRe.FindStringSubmatch(reportString)
	
	if len(csstrMatch) > 1 {
		c.Csstr = csstrMatch[1] // Update Global Token
		fmt.Println("[Masdar] ✅ LOGIN SUCCESS. New Token:", c.Csstr)
	} else {
		fallbackRe := regexp.MustCompile(`["']csstr["']\s*[:=]\s*["']?([^"']+)["']?`)
		match2 := fallbackRe.FindStringSubmatch(reportString)
		if len(match2) > 1 {
			c.Csstr = match2[1]
			fmt.Println("[Masdar] ✅ LOGIN SUCCESS (Fallback). New Token:", c.Csstr)
		} else {
			return errors.New("failed to extract csstr token")
		}
	}

	return nil
}

// ---------------------- API CALLS (With Smart Retry) ----------------------

func (c *Client) GetSMSLogs() ([]byte, error) {
	// First check
	if c.Csstr == "" {
		if err := c.ForceRelogin(""); err != nil {
			return nil, err
		}
	}

	// Capture token *before* request for comparison later
	currentToken := c.Csstr

	// --- Request Logic ---
	now := time.Now()
	todayDate := now.Format("2006-01-02")
	fdate1 := todayDate + " 00:00:00"
	fdate2 := todayDate + " 23:59:59"

	params := url.Values{}
	params.Set("fdate1", fdate1)
	params.Set("fdate2", fdate2)
	params.Set("frange", "")
	params.Set("fclient", "")
	params.Set("fg", "0")
	params.Set("csstr", currentToken) 
	params.Set("sEcho", "1")
	params.Set("iDisplayStart", "0")
	params.Set("iDisplayLength", "100") 
	params.Set("mDataProp_0", "0") 

	finalURL := SMSApiURL + "?" + params.Encode()

	req, _ := http.NewRequest("GET", finalURL, nil)
	c.setCommonHeaders(req)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Referer", ReportsPage)
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	// --- Check for Expiry ---
	if bytes.Contains(body, []byte("<!DOCTYPE html>")) || bytes.Contains(body, []byte("<html")) {
		fmt.Println("[Masdar] Session Invalid/Expired. Initiating Lock Sequence...")
		
		// یہاں "ForceRelogin" کو پرانا ٹوکن بھیجیں
		// یہ فنکشن دیکھے گا کہ اگر ٹوکن پہلے ہی بدل چکا ہے تو یہ لاگ ان نہیں کرے گا
		err := c.ForceRelogin(currentToken) 
		if err != nil {
			return nil, err
		}

		// Recursion: اب نئے ٹوکن کے ساتھ دوبارہ ٹرائی کریں
		// چونکہ ForceRelogin نے سب کچھ روک دیا تھا، اب نیا سیشن تیار ہے
		return c.GetSMSLogs()
	}

	return cleanMasdarSMS(body)
}

func (c *Client) GetNumberStats() ([]byte, error) {
	if c.Csstr == "" {
		if err := c.ForceRelogin(""); err != nil { return nil, err }
	}
	currentToken := c.Csstr

	params := url.Values{}
	params.Set("frange", "")
	params.Set("fclient", "")
	params.Set("csstr", currentToken)
	params.Set("sEcho", "2")
	params.Set("iDisplayStart", "0")
	params.Set("iDisplayLength", "-1") 
	params.Set("sSortDir_0", "asc")

	finalURL := NumberApiURL + "?" + params.Encode()

	req, _ := http.NewRequest("GET", finalURL, nil)
	c.setCommonHeaders(req)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("Referer", BaseURL+"/ints/agent/MySMSNumbers")

	resp, err := c.HTTPClient.Do(req)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if bytes.Contains(body, []byte("<!DOCTYPE html>")) || bytes.Contains(body, []byte("<html")) {
		fmt.Println("[Masdar] Number API Session Invalid. Locking...")
		err := c.ForceRelogin(currentToken)
		if err != nil { return nil, err }
		return c.GetNumberStats()
	}

	return cleanMasdarNumbers(body)
}

func (c *Client) setCommonHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Mobile Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,ur-PK;q=0.8,ur;q=0.7")
	req.Header.Set("Connection", "keep-alive")
}

// ------------------ CLEANING FUNCTIONS ------------------

func cleanMasdarSMS(rawJSON []byte) ([]byte, error) {
	var apiResp ApiResponse
	if err := json.Unmarshal(rawJSON, &apiResp); err != nil {
		return rawJSON, nil
	}
	var cleanedRows [][]interface{}
	for _, row := range apiResp.AAData {
		if len(row) > 8 {
			msg, _ := row[5].(string)
			msg = html.UnescapeString(msg)
			msg = strings.ReplaceAll(msg, "null", "")
			newRow := []interface{}{
				row[0], row[1], row[2], row[3], msg, row[6], row[7], row[8],
			}
			cleanedRows = append(cleanedRows, newRow)
		}
	}
	apiResp.AAData = cleanedRows
	return json.Marshal(apiResp)
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
			if strings.Contains(priceHTML, "€") { currency = "€" } else if strings.Contains(priceHTML, "£") { currency = "£" }
			priceVal := "0"
			matches := rePrice.FindAllString(priceHTML, -1)
			if len(matches) > 0 { priceVal = matches[len(matches)-1] }
			priceStr := currency + " " + priceVal
			stats := row[7]
			newRow := []interface{}{
				rangeName, prefix, number, billingType, priceStr, stats,
			}
			cleanedRows = append(cleanedRows, newRow)
		}
	}
	apiResp.AAData = cleanedRows
	apiResp.ITotalRecords = len(cleanedRows)
	apiResp.ITotalDisplayRecords = len(cleanedRows)
	return json.Marshal(apiResp)
}