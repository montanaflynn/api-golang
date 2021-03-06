package scrapers

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/diadata-org/api-golang/dia"
	ws "github.com/gorilla/websocket"
)

var _HuobiSocketurl string = "wss://api.huobi.pro/ws"

type EventType struct {
	Sub  string `json:"sub,omitempty"`
	Id   string `json:"id,omitempty"`
	Pong int    `json:"pong,omitempty"`
}

type ResponseType struct {
	Id     int64       `json:"id,omitempty"`
	Status string      `json:"status,omitempty"`
	Subbed string      `json:"subbed,omitempty"`
	Ts     int64       `json:"ts,omitempty"`
	Ping   int         `json:"ping,omitempty"`
	Ch     string      `json:"ch,omitempty"`
	Tick   interface{} `json:"tick,omitempty"`
}

type HuobiScraper struct {
	wsClient *ws.Conn
	// signaling channels for session initialization and finishing
	//initDone     chan nothing
	shutdown     chan nothing
	shutdownDone chan nothing
	// error handling; to read error or closed, first acquire read lock
	// only cleanup method should hold write lock
	errorLock sync.RWMutex
	error     error
	closed    bool
	// used to keep track of trading pairs that we subscribed to
	pairScrapers map[string]*HuobiPairScraper
}

// NewHuobiScraper returns a new HuobiScraper for the given pair
func NewHuobiScraper() *HuobiScraper {

	s := &HuobiScraper{
		shutdown:     make(chan nothing),
		shutdownDone: make(chan nothing),
		pairScrapers: make(map[string]*HuobiPairScraper),
		error:        nil,
	}

	var wsDialer ws.Dialer
	SwConn, _, err := wsDialer.Dial(_HuobiSocketurl, nil)

	if err != nil {
		println(err.Error())
	}

	s.wsClient = SwConn
	go s.mainLoop()
	return s
}

// runs in a goroutine until s is closed
func (s *HuobiScraper) mainLoop() {

	for true {

		message := &ResponseType{}
		_, test_read, err := s.wsClient.NextReader()

		if err != nil {
			fmt.Println(err.Error())
		} else {

			//It has to gzip response data
			reader, _ := gzip.NewReader(test_read)
			jsonBase := json.NewDecoder(reader)
			jsonBase.Decode(message)

			// If msg is ping type, it needs to resend a pong msg to ws.
			// for avoid to disconnect it
			if message.Ping > 0 {

				a := &EventType{
					Pong: message.Ping,
				}

				if err := s.wsClient.WriteJSON(a); err != nil {
					fmt.Println(err.Error())
				}
			} else {

				if message.Status == "" {

					var splitString = strings.Split(message.Ch, ".")
					var forName = strings.ToUpper(splitString[1])
					ps, ok := s.pairScrapers[forName]

					if ok {

						md := message.Tick.(map[string]interface{})
						md_data := md["data"].([]interface{})

						for _, value := range md_data {

							md_element := value.(map[string]interface{})
							f64Price := md_element["price"].(float64)
							f64Volume := md_element["amount"].(float64)
							timeStamp := time.Now().UTC()

							if md_element["direction"] == "sell" {
								f64Volume = -f64Volume
							}

							// element id is more than int64/uint64 in size
							// leave the id in float64 format
							t := &dia.Trade{
								Symbol:         ps.pair.Symbol,
								Pair:           forName,
								Price:          f64Price,
								Volume:         f64Volume,
								Time:           timeStamp,
								ForeignTradeID: strconv.FormatFloat(md_element["id"].(float64), 'E', -1, 64),
								Source:         dia.HuobiExchange,
							}
							ps.chanTrades <- t
						}
					} else {
						log.Printf("Unknown Pair %v", forName)
					}
				}
			}
		}
	}
}

func (s *HuobiScraper) cleanup(err error) {
	s.errorLock.Lock()
	defer s.errorLock.Unlock()

	if err != nil {
		s.error = err
	}
	s.closed = true

	close(s.shutdownDone)
}

// Close closes any existing API connections, as well as channels of
// PairScrapers from calls to ScrapePair
func (s *HuobiScraper) Close() error {

	if s.closed {
		return errors.New("HuobiScraper: Already closed")
	}

	close(s.shutdown)
	<-s.shutdownDone
	s.errorLock.RLock()
	defer s.errorLock.RUnlock()
	return s.error
}

// ScrapePair returns a PairScraper that can be used to get trades for a single pair from
// this APIScraper
func (s *HuobiScraper) ScrapePair(pair dia.Pair) (PairScraper, error) {

	s.errorLock.RLock()
	defer s.errorLock.RUnlock()

	if s.error != nil {
		return nil, s.error
	}

	if s.closed {
		return nil, errors.New("HuobiScraper: Call ScrapePair on closed scraper")
	}

	ps := &HuobiPairScraper{
		parent:     s,
		pair:       pair,
		chanTrades: make(chan *dia.Trade),
	}

	s.pairScrapers[pair.ForeignName] = ps

	a := &EventType{
		Sub: "market." + strings.ToLower(pair.ForeignName) + ".trade.detail",
		Id:  "id1",
	}

	if err := s.wsClient.WriteJSON(a); err != nil {
		fmt.Println(err.Error())
	}

	return ps, nil
}

// HuobiPairScraper implements PairScraper for Huobi exchange
type HuobiPairScraper struct {
	parent     *HuobiScraper
	pair       dia.Pair
	chanTrades chan *dia.Trade
	closed     bool
}

// Close stops listening for trades of the pair associated with s
func (ps *HuobiPairScraper) Close() error {
	return nil
}

// Channel returns a channel that can be used to receive trades
func (ps *HuobiPairScraper) Channel() chan *dia.Trade {
	return ps.chanTrades
}

// Error returns an error when the channel Channel() is closed
// and nil otherwise
func (ps *HuobiPairScraper) Error() error {
	s := ps.parent
	s.errorLock.RLock()
	defer s.errorLock.RUnlock()
	return s.error
}

// Pair returns the pair this scraper is subscribed to
func (ps *HuobiPairScraper) Pair() dia.Pair {
	return ps.pair
}
