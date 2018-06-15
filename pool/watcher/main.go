package main

import (
  "log"
  //"time"
  "context"

  "regexp"
  "strconv"

  "github.com/matheusb-comp/go/pool/getvoters"
  "github.com/stellar/go/clients/horizon"

  "io/ioutil"
  "net/http"
  "encoding/json"

  // TODO: Remove
  "math/rand"
)

//const DEFAULT_POOL_ADDRESS = "GCFXD4OBX4TZ5GGBWIXLIJHTU2Z6OWVPYYU44QSKCCU7P2RGFOOHTEST"
const DEFAULT_POOL_ADDRESS = "GCCD6AJOYZCUAQLX32ZJF2MKFFAUJ53PVCFQI3RHWKL3V47QYE2BNAUT"
const DEFAULT_LOCAL_HORIZON = "http://localhost:8002"
const DEFAULT_LOCAL_GETVOTERS = "http://0.0.0.0:8082"
const DEFAULT_GETVOTERS_PATH = "/voters"

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

type Page struct {
	Links struct {
		Self horizon.Link `json:"self"`
		Next horizon.Link `json:"next"`
		Prev horizon.Link `json:"prev"`
	} `json:"_links"`

	Embedded struct {
		//Records []horizon.Effect `json:"records"`
    Records []Effect `json:"records"`
	} `json:"_embedded"`
}

func convert(s string, cur string, lim int, asc bool) string {
  // Set the URL query parameters (?arg1=v1&arg2=v2...)
  param := "?order="
  if asc {
    param += "asc&"
  } else {
    param += "desc&"
  }
  if lim > 0 {
    param += "limit=" + strconv.Itoa(lim) + "&"
  }
  if cur != "" {
    param += "cursor=" + cur
  } else {
    // Remove trailing '&'
    param = string(param[:len(param)-1])
  }
  // Regular expression to get the template ({?cursor,limit,order})
  re := regexp.MustCompile("\\{\\?[a-z,]+\\}")
  // Replace the template with the set URL query parameters
	return re.ReplaceAllLiteralString(s, param)
}

func getJSON(href string, data interface{}) error {
  // Send the GET request
  resp, err := http.Get(href)
  if err != nil {
    //log.Println("ERROR - getEffects:", err)
    return err
  }
  defer resp.Body.Close()

  // Reads all the response body (streamed on demand)
  body, err := ioutil.ReadAll(resp.Body)
  if err != nil {
    //log.Println("ERROR - ReadAll response:", err)
    return err
  }

  // Unmarshal JSON to data (a pointer), and return nil
  if err = json.Unmarshal(body, data); err != nil {
    //log.Println("ERROR - json Unmarshal:", err)
    return err
  } else {
    return nil
  }
}

func main() {
  // Variable to store the current TotalCoins
  var currTotalCoins string

  // Set the horizon network
  client := horizon.DefaultTestNetClient
	//client := horizon.DefaultPublicNetClient

  // Set Horizon to local URL
	client.URL = DEFAULT_LOCAL_HORIZON

  ctx, cancel := context.WithCancel(context.Background())
  cursor := horizon.Cursor("now")

  // Start cancel timeout
  // go func(){
  //   s := 60
  //   time.Sleep(60 * time.Second)
  //   log.Printf("=== Stop (%d seconds) ===\n", s)
  //   cancel()
  // }()

  // Setup the database connection to get the voters
  conn, err := getvoters.NewDBconn(
    "postgresql://stellar:psqlpass@localhost:5002/core",
    DEFAULT_POOL_ADDRESS,
    "lumenaut.net%")
  if err != nil {
    log.Println("ERROR - NewDBconn:", err)
    return
  }
  defer conn.Close()
  log.Println("DBconn:", conn)

  err = client.StreamLedgers(ctx, &cursor, func(l horizon.Ledger){
    log.Println("Got:", l)

    // TODO: Remove - Simulate a change in TotalCoins
    if rand.Intn(3) == 0 {
      log.Println("RANDOM! Changing currTotalCoins to 1")
      currTotalCoins = "1"
    }

    // Watch for a change in Ledger.TotalCoins (means inflation happened)
    if l.TotalCoins == currTotalCoins || currTotalCoins == "" {
      log.Println("Got in the if:", currTotalCoins, l.TotalCoins)
      currTotalCoins = l.TotalCoins
      return
    }

    // We got inflation! Stop the stream
    cancel()

    // Get the voters data (snapshot)
    data, err := conn.GetVoters()
    if err != nil {
      log.Println("ERROR - GetVoters:", err)
    }

    log.Println("Count:", data.NumVoters, "Sum:", data.NumVotes)
    // for key, p := range data.Voters {
    //   log.Println("ID:", key, "Balance:", p.Balance, "Data:", p.Data)
    // }

    // Get the effects for this ledger
    effectsURL := l.Links.Effects.Href
    if l.Links.Effects.Templated {
      effectsURL = convert(l.Links.Effects.Href, "", 200, true)
    }
    log.Println("Get", effectsURL)
    getEffects(effectsURL)
  })

  if err != nil {
    log.Println("ERROR - Stream:", err)
  }

}

func getEffects(href string) {
  var page Page
  err := getJSON(href, &page)
  if err != nil {
    log.Println("ERROR - getJSON effects:", err)
    return
  }

  log.Println("Got", len(page.Embedded.Records), "effects!")
  for _, effect := range page.Embedded.Records {
    //log.Println(effect.Type)

    handleEffect(&effect)
  }
}

func handleEffect(effect *Effect) {
  // TypeI 2 is Type "account_credited"
  if effect.TypeI != 2 {
    return
  }

  log.Println("Effect type", effect.Type, "! ID:", effect.ID)
  log.Println("Account:", effect.Account)
  log.Println("Asset:", effect.AssetType, effect.AssetCode)
  log.Println("Amount:", effect.Amount)
}
