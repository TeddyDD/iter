package iter_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"go.teddydd.me/iter"
)

type Record struct {
	ID int `json:"id"`
}

func MockAPIHandler(recordsCount int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse the request body
		var requestBody struct {
			LastSeen int `json:"lastSeen"`
			Limit    int `json:"limit"`
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Simulate fetching records based on the lastSeen value
		records := make([]Record, recordsCount)
		for i := 0; i < recordsCount; i++ {
			records[i] = Record{ID: i + 1}
		}

		// Filter the records based on the lastSeen value and limit
		filteredRecords := make([]Record, 0, requestBody.Limit)
		for _, record := range records {
			if record.ID > requestBody.LastSeen {
				filteredRecords = append(filteredRecords, record)
				if len(filteredRecords) == requestBody.Limit {
					break
				}
			}
		}

		// Write the response
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(filteredRecords); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
}

func simpleIterator(mockServer *httptest.Server) *iter.CursorIterator[int, []Record] {
	return iter.New[int, []Record](iter.Config[int, []Record]{
		HasNext: func(response []Record) (int, bool) {
			if len(response) > 0 {
				// Use the last record ID as the cursor value
				return response[len(response)-1].ID, true
			}
			// No more records available
			return 0, false
		},
		GetNext: func(request int) ([]Record, error) {
			// Send a request to the mock API server with the lastSeen cursor value
			reqBody, err := json.Marshal(struct {
				LastSeen int `json:"lastSeen"`
				Limit    int `json:"limit"`
			}{
				LastSeen: request,
				Limit:    2, // Specify the desired limit
			})
			if err != nil {
				return nil, err
			}

			resp, err := http.Post(mockServer.URL, "application/json", bytes.NewReader(reqBody))
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
			}

			// Parse the response body
			var records []Record
			if err := json.NewDecoder(resp.Body).Decode(&records); err != nil {
				return nil, err
			}

			// No need to check if more records available here, since HasNext handles it.

			return records, nil
		},
		GetFirst: func() int {
			// Return an initial cursor value
			return 0
		},
	})
}

func TestCursorIterator_ManualIteration(t *testing.T) {
	// Start a mock API server
	mockServer := httptest.NewServer(MockAPIHandler(5)) // Specify the number of records
	defer mockServer.Close()

	// Create the CursorIterator
	iterator := simpleIterator(mockServer)

	t.Run("ManualIteration", func(t *testing.T) {
		// Reset the iterator
		iterator.Reset()

		// Iterate manually using Next and Get
		var results []Record
		for iterator.Next() {
			record, err := iterator.Get()
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			t.Logf("results %+v, record %+v", results, record)
			results = append(results, record...)
		}

		// Verify the results
		expected := []Record{
			{ID: 1},
			{ID: 2},
			{ID: 3},
			{ID: 4},
			{ID: 5},
		}
		if !reflect.DeepEqual(results, expected) {
			t.Errorf("Unexpected results: got %v, want %v", results, expected)
		}
	})

	t.Run("Get on deplated iterator", func(t *testing.T) {
		results, err := iterator.Get()
		if !errors.Is(err, iter.ErrStop) {
			t.Errorf("Calling Get on deplated iterator should return ErrStop but got %+v", err)
		}
		if !reflect.DeepEqual(results, []Record{}) {
			t.Errorf("calling Get on deplated iterator should return zero value response but got %+v", results)
		}
	})

	t.Run("restart", func(t *testing.T) {
		iterator.Reset()
		results, err := iterator.Get()
		if err != nil {
			t.Errorf("expected no error, got: %s", err.Error())
		}
		if !reflect.DeepEqual(results, []Record{{1}, {2}}) {
			t.Errorf("expected first two records again got %+v", results)
		}
	})
}

func TestEmptyListFirstCall(t *testing.T) {
	mockServer := httptest.NewServer(MockAPIHandler(0)) // Specify the number of records
	defer mockServer.Close()
	iterator := simpleIterator(mockServer)

	if !iterator.Next() {
		t.Error("Next should return true on first call always")
	}

	if results, err := iterator.Get(); err != nil {
		t.Error("first call should succeed")
		if !reflect.DeepEqual(results, []Record{}) {
			t.Error("and it should return zero value result")
		}
	}

}

func TestCursorIterator_CallbackIteration(t *testing.T) {
	mockServer := httptest.NewServer(MockAPIHandler(5)) // Specify the number of records
	defer mockServer.Close()
	iterator := simpleIterator(mockServer)
	type call struct {
		expectResult []Record
		returnError  error
	}

	tests := []struct {
		name        string
		iterCalls   []call
		expectError error
	}{
		{
			name: "full iteration",
			iterCalls: []call{
				{
					expectResult: []Record{{1}, {2}},
				},
				{
					expectResult: []Record{{3}, {4}},
				},
				{
					expectResult: []Record{{5}},
				},
				{
					expectResult: []Record{},
				},
			},
			expectError: nil,
		},
		{
			name: "early exit",
			iterCalls: []call{
				{
					expectResult: []Record{{1}, {2}},
					returnError:  iter.ErrStop,
				},
			},
			expectError: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			iterator.Reset()
			i := 0

			err := iterator.Iterate(func(response []Record) error {
				t.Logf("iter callback %d response %+v", i, response)
				if i > len(tc.iterCalls)-1 {
					t.Fatalf("unexpected call %d, response is %+v", i, response)
				}
				if !reflect.DeepEqual(response, tc.iterCalls[i].expectResult) {
					t.Errorf("expected result on %d callback: %+v but got %+v", i, tc.iterCalls[i], response)
				}
				res := tc.iterCalls[i].returnError
				i++
				return res
			})
			if !errors.Is(err, tc.expectError) {
				t.Errorf("expected to get %+v got: %+v", tc.expectError, err)
			}
		})
	}

}
