package main

import (
	"fmt"
	"os"
	"context"
	"strings"
	"time"
	"container/list"
	"strconv"

	"github.com/gocolly/colly"
	textapi "github.com/AYLIEN/aylien_textapi_go"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
  "github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/aws/aws-lambda-go/lambda"
	dynamodb "github.com/aws/aws-sdk-go/service/dynamodb"
	dynamodbattribute "github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
)
const MAX_ARTICLES_PER_WEBSITE = 3
const FILTER_WORD = "deals"
const FILTER_WORD2 = "podcast"
const TABLE_NAME = "TextDigest"
const ITEM_NAME = "links.txt"
const BUCKET_NAME = "textdigest"

type NewsArticle struct {
		Title string
		Date string
		Website string
		Link string
		Summary []string
}

func webLinkValidation(link string, web_link string, counter int, l *list.List) (valid bool){
	notInList := true
	backOfList := l.Back()
	for backOfList != nil && strings.Contains(backOfList.Value.(string), web_link) {
		if(backOfList.Value.(string) == link){
			notInList = false
			break
		}
		backOfList = backOfList.Prev()
	}
	return link != "" && link != "/" && !strings.Contains(link, FILTER_WORD) && !strings.Contains(link, FILTER_WORD2) && strings.Contains(link, web_link) && notInList && counter < MAX_ARTICLES_PER_WEBSITE
}

func gatherLink(e *colly.HTMLElement, web_link string, counter *int, l *list.List){
	link, _ := e.DOM.Attr("href")
	if link != "" && link[0] == '/' {
		link = web_link + link
	}
	if webLinkValidation(link, web_link, *counter, l){
		*counter++
		l.PushBack(link)
	}
}

func HandleRequest(ctx context.Context) (string, error) {
	l := list.New()

	sess := session.Must(session.NewSessionWithOptions(session.Options{
    SharedConfigState: session.SharedConfigEnable,
	}))

	buf := aws.NewWriteAtBuffer([]byte{})

	svc := dynamodb.New(sess)
	downloader := s3manager.NewDownloader(sess)

	auth := textapi.Auth{os.Getenv("AYLIEN_APP_ID"), os.Getenv("AYLIEN_API_KEY")}
  client, err := textapi.NewClient(auth, true)
  if err != nil {
		return "The job hasn't completed", err
	}

	_, err = downloader.Download(buf,
    &s3.GetObjectInput{
        Bucket: aws.String(BUCKET_NAME),
        Key:    aws.String(ITEM_NAME),
	})
	if err != nil {
		return "The job hasn't completed", err
	}

	txt_data := string(buf.Bytes())

	web_links := strings.Split(string(txt_data), "\n")

	for _, web_link := range web_links {
		counter := 0
		c := colly.NewCollector(
			colly.AllowedDomains(strings.Split(web_link, "//")[1]),
		)

		c.OnHTML("article", func(e *colly.HTMLElement) {
			link, _ := e.DOM.Find("a").Attr("href")
			if link != "" && link[0] == '/' {
				link = web_link + link
			}
			if webLinkValidation(link, web_link, counter, l) && web_link != "https://www.apnews.com"{
				counter++
				l.PushBack(link)
			}
		})

		c.OnHTML("a[data-analytics-link='article']", func(e *colly.HTMLElement) {
			gatherLink(e, web_link, &counter, l)
		})

		c.OnHTML("a[class='post-block__title__link']", func(e *colly.HTMLElement) {
			gatherLink(e, web_link, &counter, l)
		})

		c.OnHTML("a[class='headline']", func(e *colly.HTMLElement) {
			gatherLink(e, web_link, &counter, l)
		})

		c.OnRequest(func(r *colly.Request) {
			fmt.Println("Visiting", r.URL.String())
		})

		c.Visit(web_link)
		fmt.Println("The total number of links collected is: " + strconv.Itoa(counter))
	}

	for e := l.Front(); e != nil; e = e.Next() {
		extractionParams := textapi.ExtractParams{URL: e.Value.(string), BestImage: false}
		extraction, err := client.Extract(&extractionParams)
		if err != nil {
			return "The job hasn't completed", err
		}
		extractionResponse := textapi.ExtractResponse(*extraction)
		summarizationParams := textapi.SummarizeParams{Title: extractionResponse.Title, Text: extractionResponse.Article, NumberOfSentences: 2}
		summary, err := client.Summarize(&summarizationParams)
		if err != nil {
			return "The job hasn't completed", err
		}
		summarizationResponse := textapi.SummarizeResponse(*summary)
		currentArticle := NewsArticle{Title: extractionResponse.Title, Date: time.Now().Format("01-02-2006"), Link: e.Value.(string), Summary: summarizationResponse.Sentences}

		av, err := dynamodbattribute.MarshalMap(currentArticle)
		if err != nil {
			return "The job hasn't completed", err
		}

		input := &dynamodb.PutItemInput{
			Item:      av,
			TableName: aws.String(TABLE_NAME),
		}

		_, err = svc.PutItem(input)
		if err != nil {
			return "The job hasn't completed", err
		}
		fmt.Println("Successfully added the article called'" + extractionResponse.Title + " to table " + TABLE_NAME)
	}
	return "The job has completed", nil
}

func main() {
	lambda.Start(HandleRequest)
}
