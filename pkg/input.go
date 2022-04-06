package orgcollector

import (
	"context"
	"github.com/Jeffail/benthos/v3/public/service"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/numary/go-libs/sharedauth/sharedauthjwt"
	"github.com/numary/oauth2-introspect/pkg"
	"io/ioutil"
	"net"
	"net/http"
)

type message struct {
	data *service.Message
	ch   chan error
}

type Input struct {
	path          string
	address       string
	msgs          chan message
	listener      net.Listener
	logger        *service.Logger
	introspectUrl string
}

func (i *Input) handleRequest(w http.ResponseWriter, r *http.Request) {

	ch := make(chan error, 1)
	defer close(ch)

	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	org := r.Header.Get("Organization")

	m := service.NewMessage(data)
	m.MetaSet("Organization", org)

	select {
	case <-r.Context().Done():
		return
	case i.msgs <- message{
		data: m,
		ch:   ch,
	}:
		select {
		case <-r.Context().Done():
			return
		case err := <-ch:
			if err != nil {
				i.logger.Error(err.Error())
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}
	}
}

type RecoveryHandlerLoggerFn func(...interface{})

func (fn RecoveryHandlerLoggerFn) Println(v ...interface{}) {
	fn(v...)
}

func (i *Input) Connect(ctx context.Context) error {

	var err error
	i.listener, err = net.Listen("tcp", i.address)
	if err != nil {
		return err
	}

	m := mux.NewRouter()
	m.Use(handlers.RecoveryHandler(
		handlers.RecoveryLogger(RecoveryHandlerLoggerFn(func(v ...interface{}) {
			i.logger.Errorf("Recover error: %v", v)
		})),
	))
	m.Use(oauth2introspect.NewMiddleware(oauth2introspect.NewIntrospecter(http.DefaultClient, i.introspectUrl)))
	m.Use(func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			claims, err := sharedauthjwt.ClaimsFromRequest(r)
			if err != nil {
				panic(err)
			}

			org := r.Header.Get("Organization")
			if org == "" {
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}

			for _, orgClaim := range claims.Organizations {
				if org == orgClaim.ID {
					handler.ServeHTTP(w, r)
					return
				}
			}

			i.logger.Error("Agent not allowed to access organization")
			w.WriteHeader(http.StatusForbidden)
		})
	})
	m.HandleFunc(i.path, i.handleRequest)

	go func() {
		err = http.Serve(i.listener, m)
		if err != nil {
			i.logger.Error(err.Error())
		}
	}()
	return nil
}

func (i *Input) Read(ctx context.Context) (*service.Message, service.AckFunc, error) {

	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case msg := <-i.msgs:
		return msg.data, func(ctx context.Context, err error) error {
			select {
			case msg.ch <- err:
			case <-ctx.Done():
				return ctx.Err()
			}
			return nil
		}, nil
	}

}

func (i *Input) Close(ctx context.Context) error {
	return i.listener.Close()
}

func NewInput(path, address, introspectUrl string, logger *service.Logger) *Input {
	return &Input{
		path:          path,
		address:       address,
		introspectUrl: introspectUrl,
		msgs:          make(chan message),
		logger:        logger,
	}
}

func init() {
	service.RegisterInput(
		"numary_collector",
		service.NewConfigSpec().
			Field(service.NewStringField("introspect_url")).
			Field(service.NewStringField("address").Default("0.0.0.0:4196").Optional()).
			Field(service.NewStringField("path").Default("/").Optional()),
		func(conf *service.ParsedConfig, mgr *service.Resources) (service.Input, error) {
			address, err := conf.FieldString("address")
			if err != nil {
				return nil, err
			}

			path, err := conf.FieldString("path")
			if err != nil {
				return nil, err
			}

			introspectUrl, err := conf.FieldString("introspect_url")
			if err != nil {
				return nil, err
			}

			mgr.Logger().Infof("Starting Numary input with url %s%s", address, path)

			return NewInput(path, address, introspectUrl, mgr.Logger()), nil
		},
	)
}
