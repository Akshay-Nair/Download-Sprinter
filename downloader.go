package main

import "net/http"
import "log"
import "io/ioutil"
import "os"
import "fmt"
import "strconv"

var (
	contentChannel  = make(chan Content, channelNumber)
	complete        = make(chan bool, downloaderCount)
	writerComplete  = make(chan bool, fileWriter)
	errChan         = make(chan bool)
	done            = false
	channelNumber   = 2000
	fileWriter      = 4
	downloaderCount = 4
)

//Content struct is to store the resoponse recieved from the server
type Content struct {
	Data     []byte
	Location int64
}

func getRangeEnd(start, end, interval int64) int64 {

	if start+interval > end {
		return end
	}

	return start + interval
}

//function to download the content and add it to the channel
func downloader(start, end, interval int64, url string) {

	client := &http.Client{}

	for start < end {

		req, _ := http.NewRequest("GET", url, nil)
		rangeEnd := getRangeEnd(start, end, interval)

		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, rangeEnd))

		res, err := client.Do(req)
		if err != nil {
			log.Println("error occured while performing request : ", err.Error())
			errChan <- true
			return
		}

		if res.StatusCode != 206 {

			log.Println("Got Response Code : ", res.StatusCode)
			errChan <- true
			return

		} else {
			body, err := ioutil.ReadAll(res.Body)
			if err != nil {
				log.Println("following error occured while reading the body : ", err.Error())
				errChan <- true
				return
			}

			contentChannel <- Content{Data: body, Location: start}
			start = rangeEnd + 1
			res.Body.Close()
		}
		req.Close = true
	}

	complete <- true

}

//function to write the content from channel into the file
func writerFunc(file *os.File) {
	//need to add the logic to the function to stop the function when all the content have been downloaded and written into the file
	var data Content

	for len(complete) != downloaderCount {

		select {
		case data = <-contentChannel:
			_, err := file.WriteAt(data.Data, data.Location)
			if err != nil {
				log.Println("error while writing data to the file : ", err.Error())
				errChan <- true
				return
			}
		default:
		}

	}

	select {
	case writerComplete <- true:
	default:
		log.Println("Unable to write to the channel", len(writerComplete), cap(writerComplete))
	}

	return
}

func main() {
	var i int
	var start, end int64
	var fileName = "downloadedFile" //default file name

	if len(os.Args) < 2 {
		log.Println("Download URL not Provided")
		os.Exit(0)
	}

	url := os.Args[1]

	if len(os.Args) > 2 {
		fileName = os.Args[2]
	}

	log.Println("started data download...")

	resp, err := http.Head(url)
	if err != nil {
		log.Println("Following error occured while requesting the server : ", err.Error())
		os.Exit(0)
	}

	length := resp.Header.Get("Content-Length")
	contentLen, err := strconv.ParseInt(length, 10, 64)
	if err != nil {
		log.Println("Following fetching Content-Length : ", err.Error())
		os.Exit(0)
	}

	file, _ := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE, 0755)

	start = 0
	end = -1

	for i < downloaderCount {

		start = end + 1

		if i == 3 {
			end = contentLen
		} else {
			end = int64((contentLen / int64(downloaderCount)) * (int64(i) + 1))
		}

		go downloader(start, end, int64(20000), url) //20000 byte is default interval
		go writerFunc(file)

		i += 1
	}

	for len(writerComplete) != fileWriter { //wait for the writer function to complete writing all the data

		select {
		case <-errChan:
			log.Println("Writer function encountered an error")
			return
		default:
		}

	}

	log.Println("file writing complete")
	file.Close()

}
