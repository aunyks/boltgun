package main

import (
	"bufio"
	"crypto/rand"
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"time"

	"github.com/boltdb/bolt"
	"github.com/gorilla/mux"
)

const (
	AUTHED_CLIENTS_BUCKET = "authed_clients"
	TOKEN_BYTES_LENGTH    = 32
)

type AuthClient struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type Gun struct {
	ammoFile string
	bolt     *bolt.DB
	users    []AuthClient
	router   *mux.Router
}

func openTextFile(path string) []byte {
	fileBytes, err := ioutil.ReadFile(path)
	if err != nil {
		panic("Could not read file: " + path + "\nExiting...")
	}
	return fileBytes
}

func createAuthBucket(db *bolt.DB) {
	db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(AUTHED_CLIENTS_BUCKET))
		if err != nil {
			return fmt.Errorf("Couldn't create auth clients bucket: %s", err)
		}
		return nil
	})
}

func jSONEqual(a string, b string) (bool, error) {
	var o1 interface{}
	var o2 interface{}

	var err error
	err = json.Unmarshal([]byte(a), &o1)
	if err != nil {
		log.Print(a)
		_, errorMsg := fmt.Printf("Error marshalling JSON A during comparison :: %s", err.Error())
		return false, errorMsg
	}
	err = json.Unmarshal([]byte(b), &o2)
	if err != nil {
		log.Print(b)
		_, errorMsg := fmt.Printf("Error marshalling JSON B during comparison :: %s", err.Error())
		return false, errorMsg
	}

	return reflect.DeepEqual(o1, o2), nil
}

func getAuthToken(db *bolt.DB) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		keyFound := false
		body, bodyReadErr := ioutil.ReadAll(r.Body)
		if bodyReadErr != nil {
			log.Printf("Error reading request body: %v", bodyReadErr)
			http.Error(w, "Unable to read request body!", http.StatusBadRequest)
			return
		}
		// Check to make sure that the body
		// can be read as an authclient
		var authClient AuthClient
		unmarshalErr := json.Unmarshal(body, &authClient)
		if unmarshalErr != nil {
			log.Printf("Error marshaling request body: %v", unmarshalErr)
			http.Error(w, "Invalid request body!", http.StatusBadRequest)
			return
		}
		// Search DB for matching key
		// If found, return as JSON
		// If not, return JSON error
		db.View(func(tx *bolt.Tx) error {
			// Assume bucket exists and has keys
			b := tx.Bucket([]byte(AUTHED_CLIENTS_BUCKET))
			c := b.Cursor()
			for k, v := c.First(); k != nil; k, v = c.Next() {
				// if json key = json client
				clientFound, comparisonError := jSONEqual(string(k), string(body))
				if comparisonError != nil {
					log.Fatal()
				}
				if clientFound {
					keyFound = true
					tokenResponse := struct {
						Token string `json:"token"`
					}{
						b64.StdEncoding.EncodeToString([]byte(v)),
					}
					response, marshalErr := json.Marshal(tokenResponse)
					if marshalErr != nil {
						http.Error(w, "{ \"error\": \"Unable to provide response!\" }", http.StatusInternalServerError)
					}
					w.WriteHeader(http.StatusOK)
					fmt.Fprintln(w, string(response))
				}
			}
			return nil
		})
		if keyFound == false {
			errorResponse := struct {
				ErrorMsg string `json:"error"`
			}{
				"Unable to authenticate!",
			}
			response, marshalErr := json.Marshal(errorResponse)
			if marshalErr != nil {
				http.Error(w, "{ \"error\": \"Unable to provide response!\" }", http.StatusInternalServerError)
			}
			fmt.Fprintln(w, string(response))
		}
	}
}

func updateDB(db *bolt.DB) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		requestIsAuthed := false
		body, bodyReadErr := ioutil.ReadAll(r.Body)
		if bodyReadErr != nil {
			log.Printf("Error reading request body: %v", bodyReadErr)
			http.Error(w, "Unable to read request body!", http.StatusBadRequest)
			return
		}
		// Check to make sure that the body
		// can be read as a struct
		var updateRequest struct {
			Key    string `json:"key"`
			Bucket string `json:"bucket"`
			Value  string `json:"value"`
			Token  string `json:"token"`
		}
		unmarshalErr := json.Unmarshal(body, &updateRequest)
		if unmarshalErr != nil || updateRequest.Key == "" || updateRequest.Value == "" {
			log.Printf("Error marshaling request body: %v", unmarshalErr)
			http.Error(w, "{ \"error\": \"Invalid request body!\" }", http.StatusBadRequest)
			return
		}
		db.Update(func(tx *bolt.Tx) error {
			// Open the given bucket
			b, bucketCreateErr := tx.CreateBucketIfNotExists([]byte(updateRequest.Bucket))
			if bucketCreateErr != nil {
				w.WriteHeader(http.StatusInternalServerError)
				http.Error(w, "{ \"error\": \"Unable to create or open requested bucket!\" }", http.StatusBadRequest)
				return fmt.Errorf("Unable to create or open requested bucket")
			}
			c := tx.Bucket([]byte(AUTHED_CLIENTS_BUCKET)).Cursor()
			for k, v := c.First(); k != nil && requestIsAuthed == false; k, v = c.Next() {
				// if token is valid then proceed
				// if not, stop there
				if b64.StdEncoding.EncodeToString([]byte(v)) == updateRequest.Token {
					requestIsAuthed = true
				}
			}
			if requestIsAuthed {
				txionErr := b.Put([]byte(updateRequest.Key), []byte(updateRequest.Value))
				if txionErr != nil {
					w.WriteHeader(http.StatusOK)
				} else {
					w.WriteHeader(http.StatusInternalServerError)
				}
				successResponse := struct {
					Success bool `json:"success"`
				}{
					true,
				}
				response, marshalErr := json.Marshal(successResponse)
				if marshalErr != nil {
					http.Error(w, "{ \"error\": \"Unable to provide response!\" }", http.StatusInternalServerError)
				}
				fmt.Fprintln(w, string(response))
				return txionErr
			}
			errorResponse := struct {
				ErrorMsg string `json:"error"`
			}{
				"Invalid request token!",
			}
			response, marshalErr := json.Marshal(errorResponse)
			if marshalErr != nil {
				http.Error(w, "{ \"error\": \"Unable to provide response!\" }", http.StatusInternalServerError)
			}
			fmt.Fprintln(w, string(response))
			return fmt.Errorf("Unable to complete response")
		})
	}
}

func retrieveDB(db *bolt.DB) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		requestIsAuthed := false
		body, bodyReadErr := ioutil.ReadAll(r.Body)
		if bodyReadErr != nil {
			log.Printf("Error reading request body: %v", bodyReadErr)
			http.Error(w, "{ \"error\": \"Unable to read request body!\"", http.StatusBadRequest)
			return
		}
		// Check to make sure that the body
		// can be read as a struct
		var retrieveRequest struct {
			Key    string `json:"key"`
			Bucket string `json:"bucket"`
			Token  string `json:"token"`
		}
		unmarshalErr := json.Unmarshal(body, &retrieveRequest)
		if unmarshalErr != nil || retrieveRequest.Key == "" {
			log.Printf("Error marshaling request body: %v", unmarshalErr)
			http.Error(w, "{ \"error\": \"Invalid request body!\" }", http.StatusBadRequest)
			return
		}
		db.View(func(tx *bolt.Tx) error {
			// Open the given bucket
			b := tx.Bucket([]byte(retrieveRequest.Bucket))
			// Check to see if bucket exists
			if b == nil {
				http.Error(w, "{ \"error\": \"Bucket doesn't exist!\" }", http.StatusBadRequest)
				return fmt.Errorf("Client tried to access non-existent bucket: %s", retrieveRequest.Bucket)
			}
			c := tx.Bucket([]byte(AUTHED_CLIENTS_BUCKET)).Cursor()
			for k, v := c.First(); k != nil && requestIsAuthed == false; k, v = c.Next() {
				// if token is valid then proceed
				// if not, stop there
				if b64.StdEncoding.EncodeToString([]byte(v)) == retrieveRequest.Token {
					requestIsAuthed = true
				}
			}
			if requestIsAuthed {
				v := b.Get([]byte(retrieveRequest.Key))
				if v == nil {
					w.WriteHeader(http.StatusBadRequest)
					http.Error(w, "{ \"error\": \"Error retrieving requested key!\" }", http.StatusBadRequest)
					return fmt.Errorf("Client tried to delete invalid key: %s", retrieveRequest.Key)
				}
				w.WriteHeader(http.StatusOK)
				successResponse := struct {
					Value string `json:"value"`
				}{
					string(v),
				}
				response, marshalErr := json.Marshal(successResponse)
				if marshalErr != nil {
					http.Error(w, "{ \"error\": \"Unable to provide response!\" }", http.StatusInternalServerError)
				}
				fmt.Fprintln(w, string(response))
				return nil
			}
			errorResponse := struct {
				ErrorMsg string `json:"error"`
			}{
				"Invalid request token!",
			}
			response, marshalErr := json.Marshal(errorResponse)
			if marshalErr != nil {
				http.Error(w, "{ \"error\": \"Unable to provide response!\" }", http.StatusInternalServerError)
			}
			fmt.Fprintln(w, string(response))
			return fmt.Errorf("Unable to complete response")
		})
	}
}

func removeDB(db *bolt.DB) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		requestIsAuthed := false
		body, bodyReadErr := ioutil.ReadAll(r.Body)
		if bodyReadErr != nil {
			log.Printf("Error reading request body: %v", bodyReadErr)
			http.Error(w, "{ \"error\": \"Unable to read request body!\"", http.StatusBadRequest)
			return
		}
		// Check to make sure that the body
		// can be read as a struct
		var removeRequest struct {
			Key    string `json:"key"`
			Bucket string `json:"bucket"`
			Token  string `json:"token"`
		}
		unmarshalErr := json.Unmarshal(body, &removeRequest)
		if unmarshalErr != nil || removeRequest.Key == "" {
			log.Printf("Error marshaling request body: %v", unmarshalErr)
			http.Error(w, "{ \"error\": \"Invalid request body!\" }", http.StatusBadRequest)
			return
		}
		db.Update(func(tx *bolt.Tx) error {
			// Open the given bucket
			b := tx.Bucket([]byte(removeRequest.Bucket))
			// Check to see if bucket exists
			if b == nil {
				http.Error(w, "{ \"error\": \"Bucket doesn't exist!\" }", http.StatusBadRequest)
				return fmt.Errorf("Client tried to access non-existent bucket: %s", removeRequest.Bucket)
			}
			c := tx.Bucket([]byte(AUTHED_CLIENTS_BUCKET)).Cursor()
			for k, v := c.First(); k != nil && requestIsAuthed == false; k, v = c.Next() {
				// if token is valid then proceed
				// if not, stop there
				if b64.StdEncoding.EncodeToString([]byte(v)) == removeRequest.Token {
					requestIsAuthed = true
				}
			}
			if requestIsAuthed {
				deleteErr := b.Delete([]byte(removeRequest.Key))
				if deleteErr != nil {
					w.WriteHeader(http.StatusBadRequest)
					http.Error(w, "{ \"error\": \"Error deleting requested key!\" }", http.StatusBadRequest)
					return fmt.Errorf("Client tried to delete invalid key")
				}
				w.WriteHeader(http.StatusOK)
				successResponse := struct {
					Success bool `json:"success"`
				}{
					true,
				}
				response, marshalErr := json.Marshal(successResponse)
				if marshalErr != nil {
					http.Error(w, "{ \"error\": \"Unable to provide response!\" }", http.StatusBadRequest)
				}
				fmt.Fprintln(w, string(response))
				return nil
			}
			errorResponse := struct {
				ErrorMsg string `json:"error"`
			}{
				"Invalid request token!",
			}
			response, marshalErr := json.Marshal(errorResponse)
			if marshalErr != nil {
				http.Error(w, "{ \"error\": \"Unable to provide response!\" }", http.StatusInternalServerError)
			}
			fmt.Fprintln(w, string(response))
			return fmt.Errorf("Unable to complete response")
		})
	}
}

// Init configures and initializes a BoltGun instance to be launched at a later time.
func Init(ammoFilePath string, dbFilePath string) *Gun {
	// Open ammo file from directory
	ammoFileContents := openTextFile(ammoFilePath)
	// Parse JSON into AuthClients
	authenticatedClients := make([]AuthClient, 0)
	json.Unmarshal(ammoFileContents, &authenticatedClients)
	// Connect to / open db file
	db, err := bolt.Open(dbFilePath, 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	// For each authed user / client, assign them an auth token if don't have one
	createAuthBucket(db)
	for i := range authenticatedClients {
		// Get key used to identify a client with their token
		clientKey, jsonErr := json.Marshal(authenticatedClients[i])
		if jsonErr != nil {
			log.Fatal("Error storing auth token in DB.")
		}
		db.Update(func(tx *bolt.Tx) error {
			// Open the auth bucket
			b := tx.Bucket([]byte(AUTHED_CLIENTS_BUCKET))
			// Generate a client token to be associated with the key
			clientToken := make([]byte, TOKEN_BYTES_LENGTH)
			_, tokenErr := io.ReadFull(rand.Reader, clientToken)
			if tokenErr != nil {
				log.Fatal("Error generating token.")
			}
			// Store the key with its associated token
			txionErr := b.Put(clientKey, clientToken)
			return txionErr
		})
	}
	// Create router
	router := mux.NewRouter()
	router.HandleFunc("/authenticate", getAuthToken(db)).
		Methods("POST")
	router.HandleFunc("/update", updateDB(db)).
		Methods("POST")
	router.HandleFunc("/retrieve", retrieveDB(db)).
		Methods("POST")
	router.HandleFunc("/remove", removeDB(db)).
		Methods("POST")
	// Init Boltgun instance
	return &Gun{
		ammoFilePath,
		db,
		authenticatedClients,
		router,
	}
}

// Fire launches the BoltGun instance. It binds BoltGun to the given port and backs up the database to the given file location
func (g *Gun) Fire(port int64, backupFileLoc string) {
	actualPort := ":" + strconv.FormatInt(port, 10)
	ticker := time.NewTicker(2 * 60 * time.Second)
	quit := make(chan struct{})
	defer close(quit)
	if backupFileLoc != "" {
		go func() {
			for {
				select {
				case <-ticker.C:
					g.bolt.View(func(tx *bolt.Tx) error {
						var file *os.File
						var openErr error
						if _, err := os.Stat(backupFileLoc); !os.IsNotExist(err) {
							file, openErr = os.Open(backupFileLoc)
						} else {
							file, openErr = os.Create(backupFileLoc)
						}
						w := bufio.NewWriter(file)
						fmt.Println("Backing up DB")
						_, writeErr := tx.WriteTo(w)
						if openErr != nil {
							fmt.Println("Error opening backup DB file")
							return openErr
						}
						return writeErr
					})
				case <-quit:
					ticker.Stop()
					return
				}
			}
		}()
	}
	http.ListenAndServe(actualPort, g.router)
	defer g.bolt.Close()
}
