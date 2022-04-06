package orgcollector

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/Jeffail/benthos/v3/public/service"
	"github.com/golang-jwt/jwt/v4"
	"net/http"
	"time"
)

type RoundTripper struct {
	underlying  http.RoundTripper
	authUrl     string
	tokenSource string
	token       string
	claims      jwt.MapClaims
}

func (r *RoundTripper) refreshToken(ctx context.Context) error {
	req, err := http.NewRequest("POST", r.authUrl, bytes.NewBufferString(fmt.Sprintf(`{"strategy": "m2m", "token": "%s"}`, r.tokenSource)))
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")

	httpResponse, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	if httpResponse.StatusCode < 200 || httpResponse.StatusCode > 299 {
		return fmt.Errorf("unexpected status code %d when trying to get token", httpResponse.StatusCode)
	}

	type Response struct {
		Data struct {
			JWT string `json:"jwt"`
		} `json:"data"`
	}
	res := &Response{}
	err = json.NewDecoder(httpResponse.Body).Decode(res)
	if err != nil {
		return err
	}

	claims := jwt.MapClaims{}
	parser := jwt.NewParser()
	_, _, err = parser.ParseUnverified(res.Data.JWT, &claims)
	if err != nil {
		return err
	}
	r.token = res.Data.JWT
	r.claims = claims

	return nil
}

func (r *RoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	if !r.claims.VerifyExpiresAt(time.Now().Add(10*time.Second).Unix(), true) {
		err := r.refreshToken(request.Context())
		if err != nil {
			return nil, err
		}
	}
	request.Header.Set("Authorization", "Bearer "+r.token)

	rsp, err := r.underlying.RoundTrip(request)
	if err != nil {
		return nil, err
	}

	if rsp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Unexpected status code %d", rsp.StatusCode)
	}
	return rsp, nil
}

var _ http.RoundTripper = &RoundTripper{}

func NewRoundTripper(transport http.RoundTripper, authUrl string, token string) *RoundTripper {
	return &RoundTripper{
		claims:      jwt.MapClaims{},
		underlying:  transport,
		authUrl:     authUrl,
		tokenSource: token,
	}
}

type Output struct {
	httpClient   *http.Client
	url          string
	organization string
}

func (c *Output) Connect(ctx context.Context) error {
	return nil
}

func (c *Output) Write(ctx context.Context, message *service.Message) error {

	data, err := message.AsBytes()
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", c.url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)

	err = message.MetaWalk(func(key string, value string) error {
		req.Header.Set(key, value)
		return nil
	})
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Organization", c.organization)

	rsp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}

	if rsp.StatusCode < 200 || rsp.StatusCode > 299 {
		return fmt.Errorf("unexpected status code %d", rsp.StatusCode)
	}

	return nil
}

func (c *Output) Close(ctx context.Context) error {
	return nil
}

var _ service.Output = &Output{}

func NewOutput(httpClient *http.Client, url string, org string) *Output {
	return &Output{
		httpClient:   httpClient,
		url:          url,
		organization: org,
	}
}

func init() {
	service.RegisterOutput(
		"numary_collector",
		service.NewConfigSpec().
			Field(service.NewStringField("url")).
			Field(service.NewStringField("organization")).
			Field(service.NewObjectField("auth",
				service.NewStringField("url"),
				service.NewStringField("token"),
			)).
			Field(service.NewObjectField("tls",
				service.NewBoolField("skip_cert_verify").Default(false).Optional(),
			).Optional()),
		func(conf *service.ParsedConfig, mgr *service.Resources) (out service.Output, maxInFlight int, err error) {
			url, err := conf.FieldString("url")
			if err != nil {
				return nil, 0, err
			}

			skipCertVerify, err := conf.FieldBool("tls", "skip_cert_verify")
			if err != nil {
				return nil, 0, err
			}

			o, err := conf.FieldString("organization")
			if err != nil {
				return nil, 0, err
			}

			authUrl, err := conf.FieldString("auth", "url")
			if err != nil {
				return nil, 0, err
			}

			token, err := conf.FieldString("auth", "token")
			if err != nil {
				return nil, 0, err
			}

			httpTransport := &http.Transport{}
			if skipCertVerify {
				httpTransport.TLSClientConfig = &tls.Config{
					InsecureSkipVerify: skipCertVerify,
				}
			}

			httpClient := &http.Client{
				Transport: NewRoundTripper(httpTransport, authUrl, token),
			}

			return NewOutput(httpClient, url, o), 10, nil
		},
	)
}
