package main

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"log"
	"time"
	"regexp"
	"math/rand"
)

type paramCheck struct {
	url   string
	param string
}

var transport = &http.Transport{
	TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	DialContext: (&net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: time.Second,
		DualStack: true,
	}).DialContext,
}

var httpClient = &http.Client{
	Transport: transport,
}

const (
	redColor    = "\033[31m"
	greenColor  = "\033[32m"
	resetColor  = "\033[0m"
)

// 每组参数的数量
const ParamsPerGroup = 30


func main() {
	// 创建一个日志文件，如果文件不存在则创建，追加内容到文件末尾
	logFile, err := os.OpenFile("log/logfile.txt", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()

	// 设置日志输出到文件
	log.SetOutput(logFile)
	// 从文件中读取参数
	params, _ := readParamsFromFile("params.txt")

	httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	sc := bufio.NewScanner(os.Stdin)

	initialChecks := make(chan paramCheck, 40)

	done := makePool(initialChecks, func(c paramCheck, output chan paramCheck) {
		reflected, xssParams, err := checkReflected(c.url)
		if err != nil {
			return
		}

		if len(reflected) == 0 {
			return
		}
		parsedURL, _ := url.Parse(c.url)

		outputURL := fmt.Sprintf("%s://%s%s", parsedURL.Scheme, parsedURL.Host, parsedURL.Path)

		fmt.Printf("URL: %s Param: %s \n", outputURL , reflected)



		removeParamsUrl, err := removeParamsWithPattern(c.url, `z123`)
		if err != nil {
		}


		finalURL := appendParamsToURL(removeParamsUrl, xssParams)
		file, _ := os.OpenFile("check/shuf.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		defer file.Close()
		_, _ = file.Write([]byte(finalURL + "\n"))

	})

	for sc.Scan() {
		originalURL := sc.Text()
		modifiedURLs := modifyURLParameters(originalURL, params)

		for _, modifiedURL := range modifiedURLs {
			//fmt.Println(modifiedURL)
			initialChecks <- paramCheck{url: modifiedURL}
		}
	}

	close(initialChecks)
	<-done
	fmt.Printf("\x1b[32m检测完成\x1b[0m 查看\x1b[34m fuzz.txt\x1b[0m 文件\n")
}


func checkReflected(targetURL string) ([]string, []string, error) {
	out := make([]string, 0)
	params := make([]string, 0)
	maxRetries := 50


	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequest("GET", targetURL, nil)
		if err != nil {
			return out,params, err
		}
		
		//req.Header.Set("Accept-Encoding", "gzip, deflate, br")
		req.Header.Set("Accept", "*/*")
		req.Header.Set("Accept-Language", "en-US;q=0.9,en;q=0.8")
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.5845.111 Safari/537.36")
		req.Header.Set("Connection", "close")
		req.Header.Set("Cache-Control", "max-age=0")

		resp, err := httpClient.Do(req)
		if err != nil {
			return out,params, err
		}

		
		//fmt.Printf("Status Code: %d  Request URL: %s\n", resp.StatusCode, targetURL)
		log.Printf("Status Code: %d  Request URL: %s\n", resp.StatusCode, targetURL)

		if resp.StatusCode == 429 {
			// Sleep for a short duration before retrying
			fmt.Printf("%sRate limit exceeded. Sleeping for 2 minutes before retrying...%s\n", redColor, resetColor)
			time.Sleep(2 * time.Minute)
			fmt.Printf("%sResuming testing...%s\n", greenColor, resetColor)
			continue
		}

		if resp.StatusCode == http.StatusServiceUnavailable && attempt < maxRetries {
			// Sleep for a short duration before retrying
			time.Sleep(500 * time.Millisecond)
			continue
		}

		if resp.Body == nil {
			return out,params, err
		}
		defer resp.Body.Close()

		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return out,params, err
		}

		if strings.HasPrefix(resp.Status, "3") || (resp.Header.Get("Content-Type") != "" && !strings.Contains(resp.Header.Get("Content-Type"), "html")) {
			return out,params, nil
		}

		body := string(b)

		if strings.Contains(body, "Type the characters you see in this image") {
			continue
		}

		u, err := url.Parse(targetURL)
		if err != nil {
			return out,params, err
		}
		
		//fmt.Println(string(body))

		for key, vv := range u.Query() {
			for _, v := range vv {
				var xxx string
				if matches := regexp.MustCompile(`x(\w+)x`).FindStringSubmatch(v); len(matches) > 1 {
    				xxx = matches[1]
					//fmt.Println(xxx)
				}
	

				if regexp.MustCompile(`(?i)1x` + xxx + `x.*?z'z`).MatchString(body) {
					out = append(out, key+"="+"z'z")
					params = append(params, key)
				}

				if regexp.MustCompile(`(?i)1x` + xxx + `x.*?z"z`).MatchString(body) {
					out = append(out, key+"="+"z\"z")
					params = append(params, key)
				}
				
				if regexp.MustCompile(`(?i)1x` + xxx + `x.*?z<z`).MatchString(body) {
					out = append(out, key+"="+"z<z")
					params = append(params, key)
				}

				if regexp.MustCompile(`(?i)1x` + xxx + `x.*?z\\"z`).MatchString(body) {
					out = append(out, key+"="+"z\\\"z")
					params = append(params, key)
				}

				if regexp.MustCompile(`(?i)1x` + xxx +`x.*?z"\s*z`).MatchString(body) {
					out = append(out, key+"="+"z\" z")
					params = append(params, key)
				}

				if regexp.MustCompile(`(?i)1x` + xxx +`x.*?z'\s*z`).MatchString(body) {
					out = append(out, key+"="+"z' z")
					params = append(params, key)
				}


			}
		}

		
		return out,params, nil
	}

	return out,params, nil
}


func removeParamsWithPattern(u string, pattern string) (string, error) {
	parsedURL, err := url.Parse(u)
	if err != nil {
		return "", fmt.Errorf("error parsing URL: %v", err)
	}

	query, err := url.ParseQuery(parsedURL.RawQuery)
	if err != nil {
		return "", fmt.Errorf("error parsing query parameters: %v", err)
	}

	//re := regexp.MustCompile(pattern)

	for key := range query {
		if strings.Contains(query.Get(key), pattern) {
			query.Del(key)
		}
	}
	parsedURL.RawQuery = query.Encode()
	return parsedURL.String(), nil
}

// 将参数添加到 URL 后面的函数
func appendParamsToURL(baseURL string, params []string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		// 处理错误
		return baseURL
	}

	// 获取查询参数
	q := u.Query()

	// 将参数添加到查询字符串中，值设置为1，但如果参数已经存在，则跳过
	for _, param := range params {
		if _, exists := q[param]; !exists {
			q.Add(param, "1")
		}
	}
	u.RawQuery = q.Encode()

	// 返回最终的 URL 字符串
	return u.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func generateRandomString(length int) string {
	rand.Seed(time.Now().UnixNano())
	charset := "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	result := make([]byte, length)
	for i := 0; i < length; i++ {
		result[i] = charset[rand.Intn(len(charset))]
	}
	return string(result)
}

func modifyURLParameters(originalURL string, params []string) []string {
	var modifiedURLs []string

	u, err := url.Parse(originalURL)
	if err != nil {
		// 处理错误...
		return modifiedURLs
	}

	if u == nil {
		return modifiedURLs
	}
	queryParams := u.Query()
	for key, vv := range queryParams {
		for _, v := range vv {
			randomString := generateRandomString(5)
			modifiedValue := v + "1x" + randomString + "xz'z\"z<z/z123\\x"
			// 在原有参数值的基础上修改
			queryParams.Set(key, modifiedValue)
			// 生成修改后的URL
			clone := *u
			clone.RawQuery = queryParams.Encode()
			modifiedURL := clone.String()
			// 打印
			
			modifiedURLs = append(modifiedURLs, modifiedURL)
			// 恢复原有参数值，以便处理下一个参数
			queryParams.Set(key, v)
		}
	}	

	// 将params中的参数按照分组取出，然后添加到URL中
	for i := 0; i < len(params); i += ParamsPerGroup {
		paramGroup := params[i:min(i+ParamsPerGroup, len(params))]

		// 复制原始URL
		newURL := *u
		queryParams := newURL.Query()

		// 将参数添加到新的URL中
		for _, param := range paramGroup {
			randomString := generateRandomString(5)
			queryParams.Add(param, "1x" + randomString + "xz'z\"z<z/z123\\x")
		}

		newURL.RawQuery = queryParams.Encode()
		modifiedURLs = append(modifiedURLs, newURL.String())
	}
	return modifiedURLs
}

type workerFunc func(paramCheck, chan paramCheck)

func makePool(input chan paramCheck, fn workerFunc) chan paramCheck {
	var wg sync.WaitGroup

	output := make(chan paramCheck)
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			for c := range input {
				fn(c, output)
			}
			wg.Done()
		}()
	}

	go func() {
		wg.Wait()
		close(output)
	}()

	return output
}

func readParamsFromFile(filename string) ([]string, error) {
	var params []string

	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		param := strings.TrimSpace(scanner.Text())
		params = append(params, param)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return params, nil
}
