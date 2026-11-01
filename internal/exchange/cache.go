package exchange

import "fmt"

type TimedCandle struct {
	Candle
	FirstAt int64
	LastAt  int64
}
type timedChartStore interface {
	TimedChart(int64, int64, int64, int64) ([]TimedCandle, error)
}
type chartCacheEntry struct {
	Used     uint64
	Key      string
	Points   []Candle
	Times    map[int64][2]int64
	Interval int64
}

func (e *Engine) ChartTag(id, from, to, interval int64) (string, error) {
	if from < 0 || to < from || to > 8640000000000000 || interval < 1 || interval > 525600 {
		return "", fail("INVALID_RANGE", "图表时间范围无效")
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	s := e.stocks[id]
	if s == nil || s.DeletedAt != 0 {
		return "", fail("NOT_FOUND", "股票不存在")
	}
	return fmt.Sprintf("\"chart-%d-%d-%d-%d-%d\"", id, s.HistoryVersion, from, to, interval), nil
}
func (e *Engine) cachedChart(id, from, to, interval int64) ([]Candle, int64, error) {
	v, err := e.cachedChartEntry(id, from, to, interval)
	return v.Points, v.Interval, err
}
func (e *Engine) cachedChartEntry(id, from, to, interval int64) (chartCacheEntry, error) {
	tag, err := e.ChartTag(id, from, to, interval)
	if err != nil {
		return chartCacheEntry{}, err
	}
	e.chartMu.Lock()
	defer e.chartMu.Unlock()
	e.chartAccess++
	if v, ok := e.chartCache[tag]; ok {
		v.Used = e.chartAccess
		e.chartCache[tag] = v
		return v, nil
	}
	result := chartCacheEntry{Used: e.chartAccess, Key: tag, Interval: interval, Times: map[int64][2]int64{}}
	if reader, ok := e.store.(timedChartStore); ok {
		rows, err := reader.TimedChart(id, from, to, interval*60000)
		if err != nil {
			return chartCacheEntry{}, err
		}
		result.Points = []Candle{}
		for _, row := range rows {
			result.Points = append(result.Points, row.Candle)
			result.Times[row.T] = [2]int64{row.FirstAt, row.LastAt}
		}
	} else {
		points, _, err := e.Chart(id, from, to, interval)
		if err != nil {
			return chartCacheEntry{}, err
		}
		result.Points = points
		for _, c := range points {
			result.Times[c.T] = [2]int64{c.T, c.T}
		}
	}
	e.chartCache[tag] = result
	for {
		size := 0
		oldest := ""
		used := e.chartAccess
		for key, value := range e.chartCache {
			size += len(value.Points)*80 + len(key)
			if value.Used <= used {
				used = value.Used
				oldest = key
			}
		}
		if len(e.chartCache) <= 24 && size <= 8*1024*1024 {
			break
		}
		delete(e.chartCache, oldest)
	}
	return result, nil
}
