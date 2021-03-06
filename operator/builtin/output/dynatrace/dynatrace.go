package dynatrace

import (
	"bytes"
//	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
    "crypto/tls"
	"net/url"
	"sync"
	"time"
    "strings"
	"github.com/observiq/stanza/entry"
	"github.com/observiq/stanza/errors"
	"github.com/observiq/stanza/operator"
	"github.com/observiq/stanza/operator/buffer"
	"github.com/observiq/stanza/operator/flusher"
	"github.com/observiq/stanza/operator/helper"
	"go.uber.org/zap"
)

const  (
        defaultssl=  false
		defaulttimestamp= false
		defaultclusterid=""
)
func init() {
	operator.Register("dynatrace_output", func() operator.Builder { return NewDynatraceOutputConfig("") })
}

// DynatraceOutputConfig creates a dynatrace output config with default values
func NewDynatraceOutputConfig(operatorID string) *DynatraceOutputConfig {
	return &DynatraceOutputConfig{
		OutputConfig:  helper.NewOutputConfig(operatorID, "dynatrace_output"),
		BufferConfig:  buffer.NewConfig(),
		FlusherConfig: flusher.NewConfig(),
		SslVerify: defaultssl,
		ClusterID : defaultclusterid,
		Injecttimestamp: defaulttimestamp,
		Timeout:       helper.NewDuration(10 * time.Second),
		MessageField:  entry.NewRecordField(),
	}
}

// DynatraceOutputConfig is the configuration of a DynatraceOutput operator
type DynatraceOutputConfig struct {
	helper.OutputConfig `yaml:",inline"`
	BufferConfig        buffer.Config  `json:"buffer" yaml:"buffer"`
	FlusherConfig       flusher.Config `json:"flusher" yaml:"flusher"`
	APIKey       string         `json:"api_key,omitempty"       yaml:"api_key,omitempty"`
    BaseURI      string        `json:"base_uri,omitempty"      yaml:"base_uri,omitempty"`
    ClusterID    string        `json:"cluster_id,omitempty"      yaml:"cluster_id,omitempty"`
	SslVerify    bool             `json:"sslverify,omitempty"      yaml:"sslverify,omitempty"`
	Injecttimestamp bool     `json:"injectTimestamp,omitempty"      yaml:"injectTimestamp,omitempty"`
	Timeout      helper.Duration `json:"timeout,omitempty"       yaml:"timeout,omitempty"`
	MessageField entry.Field     `json:"message_field,omitempty" yaml:"message_field,omitempty"`
}

// Build will build a new NewRelicOutput
func (c DynatraceOutputConfig) Build(bc operator.BuildContext) ([]operator.Operator, error) {
	outputOperator, err := c.OutputConfig.Build(bc)
	if err != nil {
		return nil, err
	}



    if strings.TrimSpace(c.APIKey) == "" {
		return nil, errors.Wrap(err, "'api_key' cannot be empty")
	} else {
	    fmt.Println("API key : "+c.APIKey)
	}

	headers, err := c.getHeaders()
	if err != nil {
		return nil, err
	}

	buffer, err := c.BufferConfig.Build(bc, c.ID())
	if err != nil {
		return nil, err
	}

	url, err := url.Parse(c.BaseURI)
	if err != nil {
		return nil, errors.Wrap(err, "'base_uri' is not a valid URL")
	}

	flusher := c.FlusherConfig.Build(bc.Logger.SugaredLogger)
	ctx, cancel := context.WithCancel(context.Background())
    tr := &http.Transport{
                    TLSClientConfig: &tls.Config{InsecureSkipVerify: !c.SslVerify},
                }
	nro := &DynatraceOutput{
		OutputOperator: outputOperator,
		buffer:         buffer,
		flusher:        flusher,
		client:         &http.Client{Transport:tr},
		headers:        headers,
		url:            url,
		timeout:        c.Timeout.Raw(),
		messageField:   c.MessageField,
		ctx:            ctx,
		cluster_id:     c.ClusterID,
		cancel:         cancel,
	}

	return []operator.Operator{nro}, nil
}

func (c DynatraceOutputConfig) getHeaders() (http.Header, error) {
	headers := http.Header{
				"accept": []string{"application/json; charset=utf-8"},
				"Content-Type": []string{"application/json; charset=utf-8"},
	}

	if c.APIKey == ""  {
		return nil, fmt.Errorf("api_key'  is required")
	} else if c.APIKey != "" {
	    var token string
	    token="Api-Token "+c.APIKey
		headers["Authorization"] = []string{token}
	}

	return headers, nil
}

// DynatraceOutput is an operator that sends entries to the Dynatrace Logs platform
type DynatraceOutput struct {
	helper.OutputOperator
	buffer  buffer.Buffer
	flusher *flusher.Flusher
	client       *http.Client
	url          *url.URL
	cluster_id   string
	headers      http.Header
	timeout      time.Duration
    messageField entry.Field
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// Start tests the connection to Dynatrace and begins flushing entries
func (nro *DynatraceOutput) Start() error {
	//if err := nro.testConnection(); err != nil {
	//	return fmt.Errorf("test connection: %s", err)
//	}

	nro.wg.Add(1)
	go func() {
		defer nro.wg.Done()
		nro.feedFlusher(nro.ctx)
	}()

	return nil
}

// Stop tells the DynatraceOutput to stop gracefully
func (nro *DynatraceOutput) Stop() error {
	nro.cancel()
	nro.wg.Wait()
	nro.flusher.Stop()
	return nro.buffer.Close()
}

// Process adds an entry to the output's buffer
func (nro *DynatraceOutput) Process(ctx context.Context, entry *entry.Entry) error {
	return nro.buffer.Add(ctx, entry)
}

func (nro *DynatraceOutput) testConnection() error {
	ctx, cancel := context.WithTimeout(context.Background(), nro.timeout)
	defer cancel()

	req, err := nro.newRequest(ctx, nil)
	if err != nil {
		return err
	}

	res, err := nro.client.Do(req)
	if err != nil {
		return err
	}

	return nro.handleResponse(res)
}

func (nro *DynatraceOutput) feedFlusher(ctx context.Context) {
	for {
		entries, clearer, err := nro.buffer.ReadChunk(ctx)
		if err != nil && err == context.Canceled {
			return
		} else if err != nil {
			nro.Errorf("Failed to read chunk", zap.Error(err))
			continue
		}

		nro.flusher.Do(func(ctx context.Context) error {
			req, err := nro.newRequest(ctx, entries)
			if err != nil {
				nro.Errorw("Failed to create request from payload", zap.Error(err))
				// drop these logs because we couldn't creat a request and a retry won't help
				if err := clearer.MarkAllAsFlushed(); err != nil {
					nro.Errorf("Failed to mark entries as flushed after failing to create a request", zap.Error(err))
				}
				return nil
			}


			res, err := nro.client.Do(req)
			if err != nil {
				return err
			}

			if err := nro.handleResponse(res); err != nil {
				return err
			}

			if err = clearer.MarkAllAsFlushed(); err != nil {
				nro.Errorw("Failed to mark entries as flushed", zap.Error(err))
			}
			return nil
		})
	}
}

// newRequest creates a new http.Request with the given context and entries
func (nro *DynatraceOutput) newRequest(ctx context.Context, entries []*entry.Entry) (*http.Request, error) {
	payload := LogPayloadFromEntries(entries, nro.messageField,nro.cluster_id)

	var buf bytes.Buffer
	//wr := gzip.NewWriter(&buf)
	enc := json.NewEncoder(&buf)

	if err := enc.Encode(payload); err != nil {
		return nil, errors.Wrap(err, "encode payload")
	}
	//if err := wr.Close(); err != nil {
	//	return nil, err
	//}

      fmt.Println("opayload =",buf.String())
	req, err := http.NewRequestWithContext(ctx, "POST", nro.url.String(), &buf)
	if err != nil {
		return nil, err
	}
	req.Header = nro.headers

	return req, nil
}

func (nro *DynatraceOutput) handleResponse(res *http.Response) error {
	if !(res.StatusCode >= 200 && res.StatusCode < 300) {
		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return errors.NewError("unexpected status code", "", "status", res.Status)
		} else {
			if err := res.Body.Close(); err != nil {
				nro.Errorf(err.Error())
			}
			return errors.NewError("unexpected status code", "", "status", res.Status, "body", string(body))
		}
	}
	return res.Body.Close()
}
