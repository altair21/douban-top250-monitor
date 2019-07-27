package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"io/ioutil"
	"net/http"
	"net/smtp"
	"os"
	"path"
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
const localFilmsPath = "./top250.json"

const JoinBoard = "新上榜"
const LeaveBoard = "下榜"

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
	fmt.Printf("refreshing top250......\n")
	start := 0
	var res []*filmInfo
	for {
		url := fmt.Sprintf("%s/top250?start=%d&filter=", host, start)
		resp, err := http.Get(url)
		if err != nil {
			panic("refreshTop250 failed when do request: " + err.Error())
		}
		defer resp.Body.Close()
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
	fmt.Printf("refresh top250 finished.\n")
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
		fmt.Printf("send mail failed: %v\n", err)
		err = mkdir("./logs/")
		if err != nil {
			fmt.Printf("mkdir `logs` failed: %v\n", err)
		}
		err = ioutil.WriteFile(path.Join("logs", fmt.Sprintf("error_%s.txt", time.Now().Format(time.RFC3339))), []byte(content), os.ModePerm)
		if err != nil {
			fmt.Printf("write error log failed: %v\ncontent: %s\n", err, content)
		}
	}
	fmt.Println("send email succeed!")
	return nil
}

func generateFilmLog(logType string, number int, film *filmInfo) string {
	return fmt.Sprintf("【%s】%d - %s %.1f (%d人评价） %s", logType, number, film.Title, film.Rating, film.RatingNumbers, film.Inq)
}

func compareFilms(oldFilms, newFilms []*filmInfo) (hasUpdate bool, content string) {
	fmt.Printf("comparing files......\n")
	logs := []string{}
	if len(oldFilms) == 0 {
		return false, ""
	}
	for i, oldFilm := range oldFilms {
		found := false
		for _, newFilm := range newFilms {
			if oldFilm.URL == newFilm.URL {
				found = true
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

	fmt.Printf("compare files finished.\n")
	return len(logs) > 0, strings.Join(logs, "\n")
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
			fmt.Println(str)
			sendMail(receiver, "豆瓣电影Top250监测程序异常", str)
		}
	}()
	for {
		localFilms := readLocalFilms(localFilmsPath)
		newFilms := refreshTop250()
		hasUpdate, content := compareFilms(localFilms, newFilms)
		if hasUpdate {
			fmt.Printf("has update, sending email......\n")
			err := sendMail(receiver, "豆瓣电影Top250监测到变化", content)
			if err != nil {
				fmt.Printf("top250 has update but send mail failed: %v\n", err)
			}
			err = mkdir("./logs/")
			if err != nil {
				fmt.Printf("mkdir `logs` failed: %v\n", err)
			}
			err = ioutil.WriteFile(path.Join("logs", fmt.Sprintf("log_%s.txt", time.Now().Format(time.RFC3339))), []byte(content), os.ModePerm)
			if err != nil {
				fmt.Printf("write log failed: %v\ncontent: %s\n", err, content)
			}
		}
		fmt.Printf("no update.\n")
		b, err := json.Marshal(&newFilms)
		if err != nil {
			panic("marshal new films failed: " + err.Error())
		}
		err = ioutil.WriteFile(localFilmsPath, b, os.ModePerm)
		if err != nil {
			panic("update local films failed: " + err.Error())
		}

		time.Sleep(8 * time.Hour)
	}
}
