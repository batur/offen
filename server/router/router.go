package router

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/m90/go-thunk"
	logrusmiddleware "github.com/offen/logrus-middleware"
	"github.com/offen/offen/server/persistence"
	httputil "github.com/offen/offen/server/shared/http"
	"github.com/sirupsen/logrus"
)

type router struct {
	db           persistence.Database
	logger       *logrus.Logger
	secureCookie bool
}

func (rt *router) logError(err error, message string) {
	if rt.logger != nil {
		rt.logger.WithError(err).Error(message)
	}
}

func (rt *router) userCookie(userID string) *http.Cookie {
	return &http.Cookie{
		Name:     cookieKey,
		Value:    userID,
		Expires:  time.Now().Add(time.Hour * 24 * 90),
		HttpOnly: true,
		Secure:   rt.secureCookie,
	}
}

type contextKey int

const (
	cookieKey                   = "user"
	contextKeyCookie contextKey = iota
)

func (rt *router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/exchange":
		switch r.Method {
		case http.MethodGet:
			rt.getPublicKey(w, r)
		case http.MethodPost:
			rt.postUserSecret(w, r)
		default:
			httputil.RespondWithJSONError(w, errors.New("Method not allowed"), http.StatusMethodNotAllowed)
		}
	case "/accounts":
		switch r.Method {
		case http.MethodGet:
			rt.getAccount(w, r)
		default:
			httputil.RespondWithJSONError(w, errors.New("Method not allowed"), http.StatusMethodNotAllowed)
		}
	case "/events":
		c, err := r.Cookie(cookieKey)
		if err != nil {
			httputil.RespondWithJSONError(w, err, http.StatusBadRequest)
			return
		}
		if c.Value == "" {
			httputil.RespondWithJSONError(w, errors.New("received blank user identifier"), http.StatusBadRequest)
			return
		}

		r = r.WithContext(
			context.WithValue(r.Context(), contextKeyCookie, c.Value),
		)

		switch r.Method {
		case http.MethodGet:
			rt.getEvents(w, r)
		case http.MethodPost:
			rt.postEvents(w, r)
		default:
			httputil.RespondWithJSONError(w, errors.New("Method not allowed"), http.StatusMethodNotAllowed)
		}
	case "/status":
		rt.status(w, r)
	default:
		httputil.RespondWithJSONError(w, errors.New("Not found"), http.StatusNotFound)
	}
}

// New creates a new application router that reads and writes data
// to the given database implementation. In the context of the application
// this expects to be the only top level router in charge of handling all
// incoming HTTP requests.
func New(db persistence.Database, logger *logrus.Logger, secureCookie bool, origin string) http.Handler {
	router := &router{db, logger, secureCookie}
	withContentType := httputil.ContentTypeMiddleware(router, "application/json")
	l := logrusmiddleware.Middleware{
		Logger: logger,
	}
	withLogging := l.Handler(withContentType, "")
	withDNT := httputil.DoNotTrackMiddleware(withLogging)
	withCORS := httputil.CorsMiddleware(withDNT, origin)
	withRecovery := thunk.HandleSafelyWith(func(err error) {
		logger.WithError(err).Error("recovered from panic")
	})(withCORS)
	return withRecovery
}
