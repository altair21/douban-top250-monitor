package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/altair21/douban-top250-monitor/logger"
	"io/ioutil"
	"net/http"
	"net/smtp"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const email = ""
const password = ""
const smtpHost = "smtp.qq.com"
const smtpPort = 25
const defaultSender = "douban-top250-monitor"
const receiver = ""

const onePageItem = 25
const host = "https://movie.douban.com"
const localFilmsPath = "top250.json"

const JoinBoard = "新上榜"
const LeaveBoard = "下榜"

var lgr = logger.NewMyLogger()

type filmInfo struct {
	Title         string  `json:"title"`
	URL           string  `json:"url"`
	Rating        float32 `json:"rating"`
	RatingNumbers int     `json:"ratingNumbers"`
	Inq           string  `json:"inq"`
}

func isExist(filePath string) bool {
	_, err := os.Stat(filePath)
	return err == nil || os.IsExist(err)
}

func mkdir(dir string) error {
	var err error
	if _, err = os.Stat(dir); os.IsNotExist(err) {
		err = os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			return fmt.Errorf("mkdir failed: '%s' not exist and cannot create: %v", dir, err)
		}
	}
	return err
}

func readLocalFilms(filePath string) []*filmInfo {
	if !isExist(filePath) {
		return []*filmInfo{}
	}
	b, err := ioutil.ReadFile(filePath)
	if err != nil {
		panic("readLocalFilms failed when read file: " + err.Error())
	}
	var res []*filmInfo
	err = json.Unmarshal(b, &res)
	if err != nil {
		panic("readLocalFilms failed when parse file: " + err.Error())
	}
	return res
}

func refreshTop250() []*filmInfo {
	lgr.Debug("refreshing top250...")
	start := 0
	var res []*filmInfo
	var respBody []byte
	for {
		for {
			url := fmt.Sprintf("%s/top250?start=%d&filter=", host, start)
			req, err := http.NewRequest(http.MethodGet, url, nil)
			if err != nil {
				panic("refreshTop250 failed when create new request: " + err.Error())
			}
			req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_14_5) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/79.0.3945.117 Safari/537.36")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				panic("refreshTop250 failed when do request: " + err.Error())
			}
			defer resp.Body.Close()

			respBody, err = ioutil.ReadAll(resp.Body)
			if err != nil {
				panic("refreshTop250 failed when read response body: " + err.Error())
			}
			resp.Body = ioutil.NopCloser(bytes.NewBuffer(respBody))

			doc, err := goquery.NewDocumentFromReader(resp.Body)
			if err != nil {
				panic("refreshTop250 failed when create new doc from reader: " + err.Error())
			}
			doc.Find("#content > div > div.article > ol > li").Each(func(i int, selection *goquery.Selection) {
				title := selection.Find("div > div.info > div.hd > a > span").Text()
				url, _ := selection.Find("div > div.info > div.hd > a").Attr("href")
				ratingStr := selection.Find("div > div.info > div.bd > div > span.rating_num").Text()
				ratingNumbersStr := selection.Find("div > div.info > div.bd > div > span:nth-child(4)").Text()
				inq := selection.Find("div > div.info > div.bd > p.quote > span").Text()

				rating, _ := strconv.ParseFloat(ratingStr, 32)
				re := regexp.MustCompile("[0-9]+")
				ratingNumbersStr = re.FindAllString(ratingNumbersStr, -1)[0]
				ratingNumber, _ := strconv.ParseInt(ratingNumbersStr, 10, 32)
				film := filmInfo{
					Title:         title,
					URL:           url,
					Rating:        float32(rating),
					RatingNumbers: int(ratingNumber),
					Inq:           inq,
				}
				res = append(res, &film)
			})

			start += onePageItem
			if start >= 250 {
				break
			}
		}
		if len(res) >= 250 {
			break
		} else {
			lgr.Errorf("refresh failed. extract `%d` items", len(res))
			fmt.Println(string(respBody))
		}
		time.Sleep(8 * time.Hour)
		res = []*filmInfo{}
	}

	lgr.Debug("refresh top250 finished.")
	return res
}

func sendMail(receiver, subject, content string) error {
	if email == "" || password == "" || smtpHost == "" || smtpPort == 0 ||
		receiver == "" || subject == "" || content == "" {
		return nil
	}
	sender := defaultSender
	auth := smtp.PlainAuth("", email, password, smtpHost)
	to := []string{receiver}
	contentType := "Content-Type: text/plain; charset=UTF-8"
	msg := []byte("To: " + strings.Join(to, ",") + "\r\nFrom: " + sender +
		"<" + email + ">\r\nSubject: " + subject + "\r\n" + contentType + "\r\n\r\n" + content)
	smtpAddr := fmt.Sprintf("%s:%d", smtpHost, smtpPort)
	err := smtp.SendMail(smtpAddr, auth, email, to, msg)
	if err != nil {
		lgr.Errorf("send email failed: %v", err)
		err = mkdir("./logs/")
		if err != nil {
			lgr.Errorf("mkdir `logs` failed: %v", err)
		}
		err = ioutil.WriteFile(path.Join("logs", fmt.Sprintf("error_%s.txt", time.Now().Format(time.RFC3339))), []byte(content), os.ModePerm)
		if err != nil {
			lgr.Errorf("write error log failed: %v\ncontent: %s\n", err, content)
		}
	} else {
		lgr.Debug("send email succeed!")
	}
	return nil
}

func generateFilmLog(logType string, number int, film *filmInfo) string {
	return fmt.Sprintf("【%s】%d - %s %.1f (%d人评价） %s", logType, number, film.Title, film.Rating, film.RatingNumbers, film.Inq)
}

func generateIndexLog(film *filmInfo, newIndex int, oldIndex int) string {
	return fmt.Sprintf("【排位变动】%s：第 %d 名 --> 第 %d 名", film.Title, oldIndex, newIndex)
}

func compareFilms(oldFilms, newFilms []*filmInfo) (hasUpdate bool, content string) {
	lgr.Debug("comparing files...")
	logs := []string{}
	if len(oldFilms) == 0 {
		return false, ""
	}
	for i, oldFilm := range oldFilms {
		found := false
		for j, newFilm := range newFilms {
			if oldFilm.URL == newFilm.URL {
				found = true
				if i != j {
					logs = append(logs, generateIndexLog(newFilm, j, i))
				}
				break
			}
		}
		if !found {
			logs = append(logs, generateFilmLog(LeaveBoard, i+1, oldFilm))
		}
	}
	if len(logs) > 0 {
		logs = append(logs, "")
	}
	for i, newFilm := range newFilms {
		found := false
		for _, oldFilm := range oldFilms {
			if oldFilm.URL == newFilm.URL {
				found = true
				break
			}
		}
		if !found {
			logs = append(logs, generateFilmLog(JoinBoard, i+1, newFilm))
		}
	}

	lgr.Debug("compare files finished.")
	return len(logs) > 0, strings.Join(logs, "\n")
}

func initialize() {
	dir, _ := filepath.Abs("../logs")
	logger.InitializeLogger(dir, "monitor", "monitor_db")
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			err, ok := r.(error)
			if !ok {
				err = errors.New("请检查服务器")
			}
			str, ok := r.(string)
			if !ok {
				str = err.Error()
			}
			lgr.Error(str)
			sendMail(receiver, "豆瓣电影Top250监测程序异常", str)
		}
	}()

	initialize()

	err := mkdir("./logs/")
	if err != nil {
		lgr.Errorf("mkdir `logs` failed: %v", err)
		return
	}
	err = mkdir("./records/")
	if err != nil {
		lgr.Errorf("mkdir `records` failed: %v", err)
		return
	}
	for {
		localFilms := readLocalFilms(localFilmsPath)
		newFilms := refreshTop250()
		hasUpdate, content := compareFilms(localFilms, newFilms)
		if hasUpdate {
			lgr.Debug("【has update】, sending email...")
			err := sendMail(receiver, "豆瓣电影Top250监测到变化", content)
			if err != nil {
				lgr.Errorf("top250 has update but send mail failed: %v", err)
			}
			err = ioutil.WriteFile(path.Join("logs", fmt.Sprintf("log_%s.txt", time.Now().Format(time.RFC3339))), []byte(content), os.ModePerm)
			if err != nil {
				lgr.Errorf("write log failed: %v\ncontent: %s\n", err, content)
			}
		} else {
			lgr.Debug("no update.")
		}
		b, err := json.Marshal(&newFilms)
		if err != nil {
			panic("marshal new films failed: " + err.Error())
		}
		err = ioutil.WriteFile(path.Join("./records/", time.Now().Format(time.RFC3339)+"-"+localFilmsPath), b, os.ModePerm)
		if err != nil {
			panic("update local films failed: " + err.Error())
		}
		err = ioutil.WriteFile(localFilmsPath, b, os.ModePerm)
		if err != nil {
			panic("update local films failed: " + err.Error())
		}

		time.Sleep(24 * time.Hour)
	}
}
