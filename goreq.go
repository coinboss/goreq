package goreq

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"time"
	"fmt"
	"reflect"
	"net/url"
)

type Request struct {
	headers     []headerTuple
	Method      string
	Uri         string
	Body        interface{}
	QueryString interface{}
	Timeout     time.Duration
	ContentType string
	Accept      string
	Host        string
	UserAgent   string
	Username    string
	Password    string
}

type Response struct {
	StatusCode int
	Body       Body
	Header     http.Header
}

type headerTuple struct {
	name  string
	value string
}

type Body struct {
	io.ReadCloser
}

type Error struct {
	timeout bool
	Err     error
}

func (e *Error) Timeout() bool {
	return e.timeout
}

func (e *Error) Error() string {
	return e.Err.Error()
}

func (b *Body) FromJsonTo(o interface{}) error {
	if body, err := ioutil.ReadAll(b); err != nil {
		return err
	} else if err := json.Unmarshal(body, o); err != nil {
		return err
	}

	return nil
}

func (b *Body) ToString() (string, error) {
	body, err := ioutil.ReadAll(b)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func concat(a, b []string) []string {
	return append(a, b...)
}

func paramParse(query interface{})(string, error) {
	var (
		v = &url.Values{}
		s = reflect.ValueOf(query)
		t = reflect.TypeOf(query)
	)

	for i := 0; i < s.NumField(); i++ {
		v.Add(strings.ToLower(t.Field(i).Name), fmt.Sprintf("%v", s.Field(i).Interface()))
	}

	return v.Encode(), nil

}

func prepareRequestBody(b interface{}) (io.Reader, error) {
	var body io.Reader

	if sb, ok := b.(string); ok {
		// treat is as text
		body = strings.NewReader(sb)
	} else if rb, ok := b.(io.Reader); ok {
		// treat is as text
		body = rb
	} else if bb, ok := b.([]byte); ok {
		//treat as byte array
		body = bytes.NewReader(bb)
	} else {
		// try to jsonify it
		j, err := json.Marshal(b)
		if err == nil {
			body = bytes.NewReader(j)
		} else {
			return nil, err
		}
	}

	return body, nil
}

func newResponse(res *http.Response) *Response {
	return &Response{StatusCode: res.StatusCode, Header: res.Header, Body: Body{res.Body}}
}

var dialer = &net.Dialer{Timeout: 1000 * time.Millisecond}
var transport = &http.Transport{Dial: dialer.Dial}

func SetConnectTimeout(duration time.Duration) {
	dialer.Timeout = duration
}

func (r *Request) AddHeader(name string, value string) {
	if r.headers == nil {
		r.headers = []headerTuple{}
	}
	r.headers = append(r.headers, headerTuple{name: name, value: value})
}

func (r Request) Do() (*Response, error) {
	var req *http.Request
	var er error

	client := &http.Client{Transport: transport}
	b, e := prepareRequestBody(r.Body)

	if e != nil {
		// there was a problem marshaling the body
		return nil, &Error{Err: e}
	}

	if strings.EqualFold(r.Method, "GET") || strings.EqualFold(r.Method, "") {
		if r.QueryString != nil {
			param, e := paramParse(r.QueryString)
			if e != nil {
				return nil, &Error{Err: e}
			}
			r.Uri = r.Uri + "?" + param
		}
		req, er = http.NewRequest(r.Method, r.Uri, nil)
	} else {
		req, er = http.NewRequest(r.Method, r.Uri, b)
	}

	if er != nil {
		// we couldn't parse the URL.
		return nil, &Error{Err: er}
	}

	if r.Username != "" && r.Password != "" {
		req.SetBasicAuth(r.Username, r.Password)
	}

	// add headers to the request
	req.Host = r.Host
	req.Header.Add("User-Agent", r.UserAgent)
	req.Header.Add("Content-Type", r.ContentType)
	req.Header.Add("Accept", r.Accept)
	if r.headers != nil {
		for _, header := range r.headers {
			req.Header.Add(header.name, header.value)
		}
	}

	timeout := false
	var timer *time.Timer
	if r.Timeout > 0 {
		timer = time.AfterFunc(r.Timeout, func() {
			transport.CancelRequest(req)
			timeout = true
		})
	}

	res, err := client.Do(req)
	if timer != nil {
		timer.Stop()
	}

	if err != nil {
		if op, ok := err.(*net.OpError); !timeout && ok {
			timeout = op.Timeout()
		}
		return nil, &Error{timeout: timeout, Err: err}
	}
	return newResponse(res), nil
}
