package main

import (
	"context"
	"crypto/tls"
	"encoding/xml"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"code.linenisgreat.com/zit/go/zit/src/alfa/errors"
	"code.linenisgreat.com/zit/go/zit/src/bravo/ui"
	"mellium.im/sasl"
	"mellium.im/xmlstream"
	"mellium.im/xmpp"
	"mellium.im/xmpp/jid"
	"mellium.im/xmpp/mux"
	"mellium.im/xmpp/stanza"
)

// MessageBody is a message stanza that contains a body. It is normally used for
// chat messages.
type MessageBody struct {
	stanza.Message
	OriginId stanza.OriginID `xml:"origin-id"`
	Body     string          `xml:"body"`
}

func runXmpp() {
	ui.Debug().Print("parsing login")
	j := jid.MustParse(login)

	ui.Debug().Print("dialing session")
	s, err := xmpp.DialClientSession(
		context.TODO(), j,
		xmpp.BindResource(),
		xmpp.StartTLS(&tls.Config{
			ServerName: j.Domain().String(),
		}),
		xmpp.SASL("", pass, sasl.ScramSha1Plus, sasl.ScramSha1, sasl.Plain),
	)
	if err != nil {
		err = errors.Wrap(err)
		ui.Debug().Printf("Error establishing a session: %q", err)
		return
	}

	ui.Debug().Print("session started")

	var doCleanup sync.Once

	cleanup := func() {
		ui.Debug().Print("sending presence unavailable")
		err = s.Send(context.TODO(), stanza.Presence{Type: stanza.UnavailablePresence}.Wrap(nil))
		if err != nil {
			ui.Debug().Printf("Error sending initial presence: %q", err)
			return
		}

		ui.Debug().Print("Closing session…")
		if err := s.Close(); err != nil {
			ui.Debug().Printf("Error closing session: %q", err)
		}
		ui.Debug().Print("Closing conn…")
		if err := s.Conn().Close(); err != nil {
			ui.Debug().Printf("Error closing connection: %q", err)
		}
	}

	defer func() {
		doCleanup.Do(cleanup)
	}()

	ui.Debug().Print("sending presence available")
	err = s.Send(context.TODO(), stanza.Presence{Type: stanza.AvailablePresence}.Wrap(nil))
	if err != nil {
		ui.Debug().Printf("Error sending initial presence: %q", err)
		return
	}

	ui.Debug().Print("sent presence")

	seenMessages := make(map[string]struct{})

	mf := mux.MessageFunc(
		stanza.ChatMessage,
		xml.Name{},
		func(msg stanza.Message, t xmlstream.TokenReadEncoder) error {
			d := xml.NewTokenDecoder(t)

			fullMsg := MessageBody{}
			err = d.DecodeElement(&fullMsg, nil)
			if err != nil && err != io.EOF {
				ui.Debug().Print("error", err)
				return nil
			}

			if fullMsg.Body == "" {
				return nil
			}

			if _, ok := seenMessages[msg.ID]; ok {
				ui.Debug().Print("already replied to message", msg.ID)
				return nil
			}

			seenMessages[msg.ID] = struct{}{}

			reply := MessageBody{
				Message: stanza.Message{
					To: msg.From.Bare(),
				},
				Body: fullMsg.Body,
			}
			ui.Debug().Printf("Replying to message %q from %s with body %q", msg.ID, reply.To, reply.Body)
			err = t.Encode(reply)
			if err != nil {
				ui.Debug().Printf("Error responding to message %q: %q", msg.ID, err)
			}
			return nil
		},
	)

	ifunc := mux.IQFunc(
		stanza.GetIQ,
		xml.Name{},
		func(msg stanza.IQ, t xmlstream.TokenReadEncoder, start *xml.StartElement) error {
			e := xml.NewEncoder(os.Stdout)
			defer e.Flush()
			xmlstream.Copy(e, t)
			return nil
		},
	)

	sm := mux.New("", mf, ifunc)

	go s.Serve(sm)

	// Listen to Ctrl+C (you can also do something else that prevents the program from exiting)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	doCleanup.Do(cleanup)
}
