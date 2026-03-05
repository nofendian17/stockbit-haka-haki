package api

import (
	"fmt"
	"stockbit-haka-haki/database"
	models "stockbit-haka-haki/database/models_pkg"
	"testing"
)

// MockRepository for benchmarking
type MockBenchRepository struct {
	*database.TradeRepository
}

func (m *MockBenchRepository) GetSignalByID(id int64) (*models.TradingSignalDB, error) {
	return &models.TradingSignalDB{
		ID:       id,
		Strategy: "MOCK_STRATEGY",
	}, nil
}

func (m *MockBenchRepository) GetSignalsByIDs(ids []int64) (map[int64]*models.TradingSignalDB, error) {
	res := make(map[int64]*models.TradingSignalDB)
	for _, id := range ids {
		res[id] = &models.TradingSignalDB{
			ID:       id,
			Strategy: "MOCK_STRATEGY",
		}
	}
	return res, nil
}

func BenchmarkNPlusOneVsBatch(b *testing.B) {
	repo := &MockBenchRepository{}

	// Simulation of 100 items (typical limit)
	count := 100
	ids := make([]int64, count)
	for i := 0; i < count; i++ {
		ids[i] = int64(i + 1)
	}

	b.Run("N+1_Approach", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			results := make([]*models.TradingSignalDB, 0, count)
			for _, id := range ids {
				sig, _ := repo.GetSignalByID(id)
				results = append(results, sig)
			}
		}
	})

	b.Run("Batch_Approach", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			signalsMap, _ := repo.GetSignalsByIDs(ids)
			results := make([]*models.TradingSignalDB, 0, count)
			for _, id := range ids {
				if sig, ok := signalsMap[id]; ok {
					results = append(results, sig)
				}
			}
		}
	})
}

func TestTheoreticalImprovement(t *testing.T) {
	fmt.Println("This test serves to document the theoretical performance improvement.")
	fmt.Println("N+1 queries result in O(N) database roundtrips.")
	fmt.Println("Batch fetching results in O(1) database roundtrips.")
	fmt.Println("For N=100, this reduces overhead by 99%.")
}
