package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type MBean struct {
	Name       string   `json:"name"`
	Attributes []string `json:"attributes"`
	DropTags   []string `json:"droptags"`
}

type Config struct {
	MBeans   []MBean `json:"mbeans"`
	Jolokia  string  `json:"jolokia"`
	Influxdb string  `json:"influxdb"`
}

type JolokiaReq struct {
	Type      string `json:"type"`
	MBean     string `json:"mbean"`
	Attribute string `json:"attribute"`
}

type JolokiaRes struct {
	Request   JolokiaReq  `json:"request"`
	Value     interface{} `json:"value"`
	Timestamp int64       `json:"timestamp"`
	Status    int         `json:"status"`
}

var config Config
var debug bool = true

func Println(a ...interface{}) {
	if debug {
		fmt.Println(a...)
	}
}

func sync(d time.Duration, f func(time.Time)) {
	for x := range time.Tick(d) {
		go f(x)
	}
}

func stats(t time.Time) {
	fmt.Println("--> %s", t)
	reqObj := []JolokiaReq{}
	for _, mbean := range config.MBeans {
		for _, attr := range mbean.Attributes {
			reqObj = append(reqObj, JolokiaReq{
				Type:      "read",
				MBean:     mbean.Name,
				Attribute: attr})
		}
	}
	Println("   REQ %v -> %s", t, reqObj)
	reqJson, _ := json.Marshal(reqObj)
	respHttp, _ := http.Post(config.Jolokia, "application/json", bytes.NewBuffer([]byte(string(reqJson))))
	respBody, _ := ioutil.ReadAll(respHttp.Body)
	var respObj []JolokiaRes
	json.Unmarshal(respBody, &respObj)
	Println("   RES %v -> %s", t, respObj)

	var buffer bytes.Buffer
	for _, respEntry := range respObj {
		if respEntry.Status == 200 {
			value := respEntry.Value
			ts := respEntry.Timestamp
			buffer.WriteString(respEntry.Request.Attribute)
			tags(respEntry.Request.MBean, &buffer)
			switch v := value.(type) {
			case string:
				buffer.WriteString(" value=")
				buffer.WriteString(v)
			case float64:
				buffer.WriteString(" value=")
				buffer.WriteString(strconv.FormatFloat(v, 'f', -1, 64))
			case map[string]interface{}:
				buffer.WriteString(" ")
				values(v, &buffer)
			default:
				Println("   ERR %v -> %v, %v", t, v, reflect.TypeOf(v))
			}
			buffer.WriteString(" ")
			buffer.WriteString(strconv.FormatInt(ts, 10))
			buffer.WriteString("\n")
		}
	}
	writeData := buffer.String()
	Println("   WTE %v -> %s", t, writeData)
	writeRespHttp, err := http.Post(config.Influxdb, "text/text", bytes.NewBuffer([]byte(string(writeData))))
	if err != nil {
		writeRespBody, _ := ioutil.ReadAll(writeRespHttp.Body)
		Println("   ERR %v -> %s", t, string(writeRespBody))
	}
}

func values(m map[string]interface{}, buffer *bytes.Buffer) {
	flag := false
	for key, value := range m {
		if flag {
			buffer.WriteString(",")
		}
		buffer.WriteString(key)
		buffer.WriteString("=")
		switch v := value.(type) {
		case string:
			buffer.WriteString(v)
		case float64:
			buffer.WriteString(strconv.FormatFloat(v, 'f', -1, 64))
		default:
			Println("   ERR -> %v, %v", v, reflect.TypeOf(v))
		}
		flag = true
	}
}

func tags(name string, buffer *bytes.Buffer) {
	v := strings.Split(name, ":")
	buffer.WriteString(",")
	buffer.WriteString("name=")
	buffer.WriteString(v[0])
	buffer.WriteString(",")
	buffer.WriteString(v[1])
}

func main() {
	debug = os.Getenv("DEBUG") == "y"
	file, e := ioutil.ReadFile("./config.json")
	if e != nil {
		fmt.Printf("File error: %v\n", e)
		os.Exit(1)
	}
	json.Unmarshal(file, &config)
	sync(10*time.Second, stats)
}
