package smtp

import (
	"errors"

	"github.com/emersion/go-sasl"
)

// ---- AUTH LOGIN (go-sasl only ships the client side) -----------------------

type loginServer struct {
	step     int
	username string
	auth     func(username, password string) error
}

func newLoginServer(auth func(username, password string) error) sasl.Server {
	return &loginServer{auth: auth}
}

// Next implements sasl.Server for the AUTH LOGIN challenge-response flow.
func (s *loginServer) Next(response []byte) (challenge []byte, done bool, err error) {
	switch s.step {
	case 0:
		s.step++
		if len(response) > 0 { // initial response carries the username
			s.username = string(response)
			s.step++
			return []byte("Password:"), false, nil
		}
		return []byte("Username:"), false, nil
	case 1:
		s.username = string(response)
		s.step++
		return []byte("Password:"), false, nil
	case 2:
		s.step++
		return nil, true, s.auth(s.username, string(response))
	}
	return nil, false, errors.New("unexpected client response")
}
