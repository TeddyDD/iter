package iter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
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

func brokenServerHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "something went wrong", http.StatusInternalServerError)
	})
}

func dontCallMeEverAgainHandler(t testing.TB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not call server")
	})
}

func testCtx(t testing.TB) context.Context {
	ctx, c := context.WithCancel(context.Background())
	t.Cleanup(c)
	return ctx
}

func simpleIterator(mockServer *httptest.Server) *iter.Cursor[int, []Record] {
	return iter.New[int, []Record](iter.Config[int, []Record]{
		HasNext: func(ctx context.Context, result []Record) (int, bool) {
			if len(result) > 0 {
				// Use the last record ID as the cursor value
				return result[len(result)-1].ID, true
			}
			// No more records available
			return 0, false
		},
		FetchNext: func(ctx context.Context, input int) ([]Record, error) {
			// Send a request to the mock API server with the lastSeen cursor value
			reqBody, err := json.Marshal(struct {
				LastSeen int `json:"lastSeen"`
				Limit    int `json:"limit"`
			}{
				LastSeen: input,
				Limit:    2, // Specify the desired limit
			})
			if err != nil {
				return nil, err
			}

			req, err := http.NewRequestWithContext(
				ctx,
				http.MethodPost,
				mockServer.URL,
				bytes.NewReader(reqBody),
			)
			if err != nil {
				return nil, err
			}

			resp, err := http.DefaultClient.Do(req)
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
		GetFirstInput: func() int {
			// Return an initial cursor value
			return 0
		},
	})
}

func brokenIterator() *iter.Cursor[int, []Record] {
	return iter.New[int, []Record](
		iter.Config[int, []Record]{
			HasNext: func(ctx context.Context, result []Record) (int, bool) {
				return 0, true
			},
			FetchNext: func(ctx context.Context, input int) ([]Record, error) {
				return nil, errors.New("nope")
			},
			GetFirstInput: func() int {
				return 1
			},
		},
	)
}

func TestCursorIterator_ManualIteration(t *testing.T) {
	// Start a mock API server
	mockServer := httptest.NewServer(MockAPIHandler(5)) // Specify the number of records
	t.Cleanup(mockServer.Close)

	// Create the CursorIterator
	iterator := simpleIterator(mockServer)

	t.Run("ManualIteration", func(t *testing.T) {
		// Reset the iterator
		iterator.Reset()
		ctx := testCtx(t)

		// Iterate manually using Next and Get
		var results []Record
		for iterator.Next() {
			record, err := iterator.Get(ctx)
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
		ctx := testCtx(t)
		results, err := iterator.Get(ctx)
		if !errors.Is(err, iter.ErrStop) {
			t.Errorf("Calling Get on deplated iterator should return ErrStop but got %+v", err)
		}
		if !reflect.DeepEqual(results, []Record{}) {
			t.Errorf("calling Get on deplated iterator should return zero value response but got %+v", results)
		}
	})

	t.Run("restart", func(t *testing.T) {
		ctx := testCtx(t)
		iterator.Reset()
		results, err := iterator.Get(ctx)
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

	ctx := testCtx(t)
	if results, err := iterator.Get(ctx); err != nil {
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
			ctx := testCtx(t)
			i := 0

			err := iterator.Iterate(ctx, func(_ context.Context, response []Record) error {
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

func TestUnexpectedServerError(t *testing.T) {
	mockServer := httptest.NewServer(brokenServerHandler())
	ctx := testCtx(t)
	defer mockServer.Close()
	iterator := simpleIterator(mockServer)
	err := iterator.Iterate(ctx, func(_ context.Context, response []Record) error {
		t.Fatalf("should not be called")
		return nil
	})
	if err == nil {
		t.Fatalf("should result in error")
	}
	if !strings.Contains(err.Error(), "unexpected status code: 500") {
		t.Fatalf("error should be unexpected status code but got %+v", err)
	}
	iterator.Reset()
	_, err2 := iterator.Get(ctx)
	if err2.Error() != err.Error() {
		t.Fatalf("error should be unexpected status code but got %+v", err2)
	}
}

func TestCancel(t *testing.T) {
	mockServer := httptest.NewServer(dontCallMeEverAgainHandler(t))
	ctx, c := context.WithCancel(context.Background())

	c()
	t.Cleanup(mockServer.Close)
	iterator := simpleIterator(mockServer)
	v, err := iterator.Get(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("context should be canceled")
	}
	if v != nil {
		t.Fatal("result should be nil")
	}
	err = iterator.Iterate(ctx, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("context should be canceled")
	}
}

func TestErrors(t *testing.T) {
	mockServer := httptest.NewServer(dontCallMeEverAgainHandler(t))
	ctx := testCtx(t)
	t.Cleanup(mockServer.Close)

	iterator := brokenIterator()
	_, err := iterator.Get(ctx)
	if err.Error() != "nope" {
		t.FailNow()
	}

	err = iterator.Iterate(ctx, nil)
	if err.Error() != "nope" {
		t.FailNow()
	}
}
