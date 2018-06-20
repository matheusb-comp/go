package main

import (
  "io"
  "os"
  "fmt"
  "log"
  "flag"
  "strings"
  "context"
  "net/http"
  "io/ioutil"
  "encoding/json"
  "github.com/jtacoma/uritemplates"
  "github.com/stellar/go/clients/horizon"
  "github.com/matheusb-comp/go/pool/getvoters"
)

// Needed because clients/horizon currently doesn't provide effects streaming
type Effect struct {
  // Base
	Links struct {
		Operation horizon.Link `json:"operation"`
		Succeeds  horizon.Link `json:"succeeds"`
		Precedes  horizon.Link `json:"precedes"`
	} `json:"_links"`
	ID        string `json:"id"`
	PT        string `json:"paging_token"`
	Account   string `json:"account"`
	Type      string `json:"type"`
	TypeI     int32  `json:"type_i"`
	CreatedAt string `json:"created_at"`
  // Asset
  AssetType string `json:"asset_type"`
	AssetCode string `json:"asset_code,omitempty"`
	Issuer    string `json:"asset_issuer,omitempty"`
  // AccountCredited and AccountDebited
  Amount string `json:"amount"`
}
type EffectsPage struct {
	Links struct {
		Self horizon.Link `json:"self"`
		Next horizon.Link `json:"next"`
		Prev horizon.Link `json:"prev"`
	} `json:"_links"`
	Embedded struct {
    Records []Effect `json:"records"`
	} `json:"_embedded"`
}

// Current state to be saved in a file in case of error
type State struct {
  Cursor string
  TotalCoins string
  Error string
  Snapshot *getvoters.Data
}

// Used to create the final JSON file with all the inflation information
type InflationData struct {
  Ledger int32
  Address string
  Credit string
  Snapshot *getvoters.Data
}

// Parameters to apply when parsing a templated Stellar URI
const DEFAULT_TEMPLATE_CURSOR = ""
const DEFAULT_TEMPLATE_ORDER = "asc"
const DEFAULT_TEMPLATE_LIMIT = 2
var templateParams = map[string]interface{}{
  "cursor": DEFAULT_TEMPLATE_CURSOR,
  "order": DEFAULT_TEMPLATE_ORDER,
  "limit": DEFAULT_TEMPLATE_LIMIT,
}

// User-defined variables
var dbUser, dbPass, dbName, dbHost, dbPort, dbConn string
var horizonURL, defaultPool, donationKey string
var errorFile, votersFile string
// Object to get the voters snapshot from
var conn *getvoters.DBconn
// Context that will be passed to the StreamLedgers function
var ctx context.Context
// Cancel function to stop the stream
var cancel context.CancelFunc
// Current state, updated every ledger
var curr State

func init() {
	// Database flags
	flag.StringVar(&dbUser, "user", "stellar",
		"PostgreSQL user name to connect as")

	flag.StringVar(&dbPass, "pass", "",
		"Password to be used if the server demands password authentication")

	flag.StringVar(&dbName, "db", "core", "The database name")

	flag.StringVar(&dbHost, "host", "localhost",
		"Name of host to connect to. If a host name begins with a slash, " +
		"it specifies Unix-domain communication rather than TCP/IP communication")

	flag.StringVar(&dbPort, "port", "5432",
		"Port number to connect to at the server host, " +
		"or socket file name extension for Unix-domain connections")

	flag.StringVar(&dbConn, "conn", "",
		"Optional custom PostgreSQL connection string. If provided, " +
		"it's used instead of the other flags")

	// Stellar flags
  flag.StringVar(&horizonURL, "horizon", "https://horizon.stellar.org",
    "URL of a horizon server to stream ledgers")

	flag.StringVar(&defaultPool, "pool",
		"GCCD6AJOYZCUAQLX32ZJF2MKFFAUJ53PVCFQI3RHWKL3V47QYE2BNAUT",
		"Default inflationdest address to use")

	flag.StringVar(&donationKey, "key", "lumenaut.net donation%",
		"Format of key for a voter data pair to mark a donation")

  // Files to save the voters snapshot and the status in case of errors
  flag.StringVar(&errorFile, "error", "error.json",
    "JSON file to store the state (cursor and total_coins) " +
    "in case of a fatal error, to allow resuming the stream")

  flag.StringVar(&votersFile, "voters", "voters.json",
    "JSON file to store the voters snapshot at the moment of inflation")
}

func main() {
  var err error
  flag.Parse()

  // Set the horizon network client and URL
	client := horizon.DefaultPublicNetClient
	client.URL = horizonURL

  // Create a connection string only if one was not supplied
	dbString := dbConn
	if len(dbString) < 1 {
		dbString = "dbname=" + dbName +
		" user=" + dbUser +
		" password=" + dbPass +
		" host=" + dbHost +
		" port=" + dbPort
	}
  // Setup the database connection to get the voters
  conn, err = getvoters.NewDBconn(dbString, defaultPool, donationKey)
  checkFatal("Create new DBconn", err, nil)
  defer conn.Close()

  // Get the current state from the file, or stream from 'now'
  err = readFileJSON(errorFile, &curr)
  if err != nil {
    curr.TotalCoins = ""
    curr.Cursor = "now"
  }

  // Prepare the context and cancel function
  ctx, cancel = context.WithCancel(context.Background())

  // -- STREAM START --
  c := horizon.Cursor(curr.Cursor)
  err = client.StreamLedgers(ctx, &c, handleLedger)
  checkFatal("Stream Ledgers", err, &curr)
}

// TODO: Remove - Simulate a change in TotalCoins
var counter int
func handleLedger(l horizon.Ledger) {
  // TODO: Remove - Simulate a change in TotalCoins
  counter++
  if counter > 3 {
    fmt.Println("RANDOM! Changing curr.totalCoins to 1")
    curr.TotalCoins = "1"
  }
  // END-Remove
  var err error

  // When inflation happens, the Ledger.TotalCoins changes
  fmt.Println("Checking ledger", l.Sequence)
  if l.TotalCoins == curr.TotalCoins || curr.TotalCoins == "" {
    // Update the current state (cursor and totalCoins)
    curr.Cursor = l.PT
    curr.TotalCoins = l.TotalCoins
    // No inflation yet, ignore this ledger
    return
  }
  // We got inflation! -- STREAM END --
  fmt.Println("Inflation!")
  if cancel != nil {
    cancel()
  }

  // Get the voters snapshot, or save the cursor in case of error
  curr.Snapshot, err = conn.GetVoters()
  checkFatal("GetVoters", err, &curr)
  fmt.Println("Voters:", curr.Snapshot.NumVoters, "- Votes:", curr.Snapshot.NumVotes)

  // Extract the effects URL for this ledger (with the params applied)
  effectsURL := l.Links.Effects.Href
  if l.Links.Effects.Templated {
    template, err := uritemplates.Parse(effectsURL)
    checkFatal("Template parse", err, &curr)
    effectsURL, err = template.Expand(templateParams)
    checkFatal("Template expand", err, &curr)
  }

  // Loop the pages until Records is an empty slice
  var credit string
  LoopPages:
    for {
      var page EffectsPage
      err = getJSON(effectsURL, &page)
      checkFatal("GET " + effectsURL, err, &curr)

      // Stop (get out of for) if the page has no effects
      if len(page.Embedded.Records) <= 0 {
        break LoopPages
      }

      // Check the page for typeI 2 (account_credited)
      for _, effect := range page.Embedded.Records {
        if effect.TypeI == 2 && effect.Account == defaultPool {
          credit = effect.Amount
          // TODO: Decide - We break after finding the first credit?
          break LoopPages
        }
      }

      // Get the next page
      effectsURL = page.Links.Next.Href
    }
  fmt.Println("Stroops received:", credit)

  // TODO: Print the final file in a better way
  err = writeFileJSON(votersFile, InflationData{
    l.Sequence,
    defaultPool,
    strings.Replace(credit, ".", "", 1),
    curr.Snapshot})
  checkFatal("Write " + votersFile, err, &curr)
  // Everything went ok, we have a functional snapshot!
  fmt.Println("Inflation snapshot successfully saved in", votersFile)
}

// Log the fatal error, save all the data in files, and exit (OS.Exit(1))
func checkFatal(msg string, err error, state *State) {
  if err != nil {
    if state != nil {
      state.Error = msg + ": " + err.Error()
      err = writeFileJSON(errorFile, state)
      if err != nil {
        log.Println("### ERROR SAVING " + errorFile + " ###")
        log.Println(state)
        log.Println("######")
      }
    }
    // Print the error message received and exit with status 1
    log.Fatalln("ERROR - " + msg + ":", err)
  }
}

// Helper functions to write JSON to a file
func writeFileJSON(name string, data interface{}) error {
  // Marshal the data to a json indented string (slice of bytes)
  b, err := json.MarshalIndent(data, "", " ")
  if err != nil {
    return err
  }
  // Save the bytes to the file (truncate if exists, create if doesn't)
  err = ioutil.WriteFile(name, b, 0666)
  if err != nil {
    return err
  }
  // Everything worked, return nil
  return nil
}

// Helper functions to read JSON from files and URLs (GET)
func readFileJSON(name string, data interface{}) error {
  f, err := os.Open(name)
  if err != nil {
    return err
  }
  defer f.Close()
  // Read an unmarshal the entire JSON file
  return readJSON(f, data)
}
func getJSON(href string, data interface{}) error {
  resp, err := http.Get(href)
  if err != nil {
    return err
  }
  defer resp.Body.Close()
  // Read and unmarashal the response body
  return readJSON(resp.Body, data)
}
func readJSON(r io.Reader, data interface{}) error {
  // Streamed on demand until EOF (stored on memory)
  b, err := ioutil.ReadAll(r)
  if err != nil {
    return err
  }
  // Unmarshal JSON to data (a pointer)
  if err = json.Unmarshal(b, data); err != nil {
    return err
  }
  // Everything worked, return nil
  return nil
}
