package main

import (
	"context"
	"errors"
	"fmt"

	"gonum.org/v1/gonum/stat/distuv"

	"math"
	"strconv"

	"github.com/gin-gonic/gin"
	redis "github.com/go-redis/redis/v9"
)

type database struct {
	client *redis.Client
}

func GetPopulationCount(ctx *gin.Context, db *database, scope string) int64 {
	key := fmt.Sprintf("%s:all:users:count", scope)
	val := db.client.PFCount(ctx, key)

	return val.Val()
}

func UpdatePopulationCount(ctx *gin.Context, db *database, scope string, id string) int64 {
	key := fmt.Sprintf("%s:all:users:count", scope)
	if err := db.client.PFAdd(ctx, key, id).Err(); err != nil {
		panic(err)
	}
	val := db.client.PFCount(ctx, key)

	return val.Val()
}

func GetActionCount(ctx *gin.Context, db *database, scope string) int64 {
	key := fmt.Sprintf("%s:all:actions:count", scope)
	val := db.client.PFCount(ctx, key)

	return val.Val()
}

func UpdateActionCount(ctx *gin.Context, db *database, scope string, action string) int64 {
	key := fmt.Sprintf("%s:all:actions:count", scope)
	if err := db.client.PFAdd(ctx, key, action).Err(); err != nil {
		panic(err)
	}
	val := db.client.PFCount(ctx, key)

	return val.Val()
}

func IncreasePopulationActionCount(ctx *gin.Context, db *database, scope string, action string) (int64, int64) {
	actionSpecificKey := fmt.Sprintf("%s:all:users:counters:distinct", scope)
	totalKey := fmt.Sprintf("%s:all:users:counters:total", scope)

	actionCount := db.client.HIncrBy(ctx, actionSpecificKey, action, 1)
	totalCount := db.client.IncrBy(ctx, totalKey, 1)

	return actionCount.Val(), totalCount.Val()
}

func IncreaseIdActionCount(ctx *gin.Context, db *database, scope string, id string, action string) (int64, int64) {
	actionSpecificKey := fmt.Sprintf("%s:single:%s:counters:distinct", scope, id)
	totalKey := fmt.Sprintf("%s:single:counters:total", scope)

	actionCount := db.client.HIncrBy(ctx, actionSpecificKey, action, 1)
	totalCount := db.client.HIncrBy(ctx, totalKey, id, 1)

	return actionCount.Val(), totalCount.Val()
}

func IncreaseActionPopulationCount(ctx *gin.Context, db *database, scope string, id string) (int64, int64) {
	idSpecificKey := fmt.Sprintf("%s:all:actions:counters:distinct", scope)
	totalKey := fmt.Sprintf("%s:all:actions:counters:total", scope)

	idCount := db.client.HIncrBy(ctx, idSpecificKey, id, 1)
	totalCount := db.client.IncrBy(ctx, totalKey, 1)

	return idCount.Val(), totalCount.Val()
}

func IncreaseActionIdCount(ctx *gin.Context, db *database, scope string, id string, action string) (int64, int64) {
	idSpecificKey := fmt.Sprintf("%s:single:action:%s:counters:distinct", scope, action)
	totalKey := fmt.Sprintf("%s:single:action:counters:total", scope)

	idCount := db.client.HIncrBy(ctx, idSpecificKey, id, 1)
	totalCount := db.client.HIncrBy(ctx, totalKey, id, 1)

	return idCount.Val(), totalCount.Val()
}

func GetCountForKey(ctx *gin.Context, db *database, key string) map[string]int64 {
	j, err := db.client.HGetAll(ctx, key).Result()
	if err != nil {
		panic(err)
	}

	var m = make(map[string]int64)

	for k, v := range j {
		i, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			fmt.Println("Can't convert v to int64", v)
		}

		m[k] = int64(math.Max(float64(i), 0.0))
	}
	return m
}

func GetGlobalCounts(ctx *gin.Context, db *database, scope string) map[string]int64 {
	key := fmt.Sprintf("%s:all:users:counters:distinct", scope)
	m := GetCountForKey(ctx, db, key)

	return m
}

func GetIdCountMap(ctx *gin.Context, db *database, scope string, id string) map[string]int64 {
	key := fmt.Sprintf("%s:single:%s:counters:distinct", scope, id)
	m := GetCountForKey(ctx, db, key)

	return m
}

func GetActionCountMap(ctx *gin.Context, db *database, scope string, action string) map[string]int64 {
	key := fmt.Sprintf("%s:single:action:%s:counters:distinct", scope, action)
	m := GetCountForKey(ctx, db, key)

	return m
}

func calculateBitsOfInfo(p, n, x float64) float64 {
	dist := distuv.Binomial{N: float64(n), P: p}

	if p == 1 {
		return 0.0
	}

	// baseline noise reduction section
	u := n * p
	s := n * p * (1 - p)

	// having a value greater than zero here allows to reduce noice, but it has the side effect of causing greater standard deviations
	// between normal users and less normal ones. generating some noise for normal users also helps having a better distribution
	// I set nbStd to 0 for now to disable the flattening for normal users, and I will likely remove it completely eventually
	// it's just nice for visualization where normal users will remain at the buttom of the graph, and extremes will skyrocket up
	const nbStd = 0
	xprime := math.Min(math.Round(u)+math.Round(s*nbStd), n)
	base := dist.Prob(xprime)
	noise := -math.Log2(base)
	// end of the baseline noise reduction section

	// real values
	y := dist.Prob(x)
	if y == 0 {
		y = 1e-5
	}
	bitsOfInfo := -math.Log2(y)

	return math.Max(bitsOfInfo-noise, 0)
}

func idCompleteBitsOfInfo(ctx *gin.Context, db *database, scope string, id string, globalMap map[string]int64, distinctMap map[string]int64, globalCount int64, n int64, nnt float64) float64 {
	// globalCount: populationTotalActionCount
	// n: idTotalActionCount
	// nnt: idNormalizeTo

	idActionBitsOfInfoHKey := fmt.Sprintf("%s:single:%s:bits:distinct", scope, id)
	idTotalBitsOfInfoKey := fmt.Sprintf("%s:single:bits:total", scope)

	var totalBitsOfInfo float64

	nFloat := float64(n)
	nnt = math.Min(nFloat, nnt)

	for action, value := range globalMap {
		var x float64
		elem, ok := distinctMap[action]
		if ok {
			x = float64(elem)
		}

		p := math.Min(float64(value), 1) / float64(globalCount)

		nX := math.Round(x * nnt / nFloat)

		bitsOfInfo := calculateBitsOfInfo(p, nnt, nX)

		db.client.HSet(ctx, idActionBitsOfInfoHKey, action, bitsOfInfo)
		totalBitsOfInfo += bitsOfInfo
	}

	db.client.HSet(ctx, idTotalBitsOfInfoKey, id, totalBitsOfInfo)

	return totalBitsOfInfo
}

func idActionBitsOfInfo(ctx *gin.Context, db *database, scope string, id string, action string, globalCount int64, b int64, n int64, x int64, nnt float64) float64 {
	// globalcount: populationTotalActionCount
	// b: populationActionCount
	// n: idTotalActionCount
	// x: idActionCount
	// nnt: idNormalizeTo

	idActionBitsOfInfoHKey := fmt.Sprintf("%s:single:%s:bits:distinct", scope, id)
	idTotalBitsOfInfoKey := fmt.Sprintf("%s:single:bits:total", scope)

	ps := db.client.HGet(ctx, idActionBitsOfInfoHKey, action).Val() // population specific bitsOfInfo
	ts := db.client.HGet(ctx, idTotalBitsOfInfoKey, id).Val()       // total bitsOfInfo

	var psf float64
	var tsf float64

	if ps == "" || ts == "" {
		return 0.0
	}

	if ps != "NaN" {
		var err error
		psf, err = strconv.ParseFloat(ps, 64)
		if err != nil {
			fmt.Println("error 12:", err)
		}
	}

	if ts != "NaN" {
		var err error
		tsf, err = strconv.ParseFloat(ts, 64)
		if err != nil {
			fmt.Println("error 13:", err)
		}
	}

	p := math.Max(float64(b), 1) / math.Max(float64(globalCount), 1)

	// Normalizing id count to a restricted range
	nnt = math.Min(float64(n), nnt)

	nX := math.Round(float64(x) * nnt / float64(n))

	// then continue with the calculation with the new values if applicable.
	bitsOfInfo := calculateBitsOfInfo(p, nnt, nX)

	db.client.HSet(ctx, idActionBitsOfInfoHKey, action, bitsOfInfo)
	totalBitsOfInfo := math.Max(tsf-psf+bitsOfInfo, 0)

	db.client.HSet(ctx, idTotalBitsOfInfoKey, id, totalBitsOfInfo)

	return totalBitsOfInfo
}

func actionCompleteBitsOfInfo(ctx *gin.Context, db *database, scope string, action string, globalMap map[string]int64, distinctMap map[string]int64, globalCount int64, n int64, nnt float64) float64 {
	// globalCount: mapGlobalActionCount
	// n: actionTotalIdCount
	// nnt: idNormalizeTo

	actionIdBitsOfInfoHKey := fmt.Sprintf("%s:single:action:%s:bits:distinct", scope, action)
	actionTotalBitsOfInfoHKey := fmt.Sprintf("%s:single:action:bits:total", scope)

	var totalBitsOfInfo float64

	nFloat := float64(n)
	nnt = math.Min(nFloat, nnt)

	for id, value := range globalMap {
		var x float64
		elem, ok := distinctMap[action]
		if ok {
			x = float64(elem)
		}

		p := math.Min(float64(value), 1) / float64(globalCount)

		nX := math.Round(x * nnt / nFloat)

		bitsOfInfo := calculateBitsOfInfo(p, nnt, nX)

		db.client.HSet(ctx, actionIdBitsOfInfoHKey, id, bitsOfInfo)
		totalBitsOfInfo += bitsOfInfo
	}

	db.client.HSet(ctx, actionTotalBitsOfInfoHKey, action, totalBitsOfInfo)

	return totalBitsOfInfo
}

func ActionIdBitsOfInfo(ctx *gin.Context, db *database, scope string, id string, action string, globalCount int64, b int64, n int64, x int64, nnt float64) float64 {
	// globalcount: actionTotalPopulationCount
	// b: actionPopulationCount
	// n: actionTotalIdCount
	// x: actionIdCount
	// nnt: idNormalizeTo

	actionIdBitsOfInfoHKey := fmt.Sprintf("%s:single:action:%s:bits:distinct", scope, action)
	actionTotalBitsOfInfoKey := fmt.Sprintf("%s:single:action:bits:total", scope)

	ps := db.client.HGet(ctx, actionIdBitsOfInfoHKey, id).Val()       // action specific bitsOfInfo
	ts := db.client.HGet(ctx, actionTotalBitsOfInfoKey, action).Val() // total bitsOfInfo for a given action

	var psf float64
	var tsf float64

	if ps == "" || ts == "" {
		return 0.0
	}

	if ps != "NaN" {
		var err error
		psf, err = strconv.ParseFloat(ps, 64)
		if err != nil {
			fmt.Println("error 12:", err)
		}
	}

	if ts != "NaN" {
		var err error
		tsf, err = strconv.ParseFloat(ts, 64)
		if err != nil {
			fmt.Println("error 13:", err)
		}
	}

	// p := math.Max(float64(b), 1) / math.Max(float64(globalCount), 1)
	p := math.Max(float64(b), 1) / math.Max(float64(globalCount), 1)

	// Normalizing id count to a restricted range
	nnt = math.Min(float64(n), nnt)

	nX := math.Round(float64(x) * nnt / float64(n))

	// then continue with the calculation with the new values if applicable.
	bitsOfInfo := calculateBitsOfInfo(p, nnt, nX)

	db.client.HSet(ctx, actionIdBitsOfInfoHKey, id, bitsOfInfo)
	totalBitsOfInfo := math.Max(tsf-psf+bitsOfInfo, 0)

	db.client.HSet(ctx, actionTotalBitsOfInfoKey, action, totalBitsOfInfo)

	return totalBitsOfInfo
}

func calculateAverageStd(ctx *gin.Context, db *database, scope string, id string) (float64, float64) {
	const SampleSize = 1200

	var values []float64
	var sum float64
	var mean, std float64

	key := fmt.Sprintf("%s:single:bits:total", scope)

	kv, err := db.client.HRandFieldWithValues(ctx, key, SampleSize).Result()

	if err != nil {
		panic(err)
	}
	if len(kv) == 0 {
		return mean, 1
	}

	for k := range kv {
		if kv[k].Value != "NaN" && kv[k].Key != id {
			f, err := strconv.ParseFloat(kv[k].Value, 32)
			if err != nil {
				fmt.Println("Can't convert v to int64", kv[k].Value)
			}
			values = append(values, f)
			sum += f
		}
	}

	if len(values) > 0 {
		mean = sum / math.Max(float64(len(values)), 1.0)
	}

	for i := 0; i < len(values); i++ {
		std += math.Pow(values[i]-mean, 2)
	}

	std = math.Sqrt(std / math.Max(float64(len(values)), 1.0))
	allBitsOfInfoAverage := fmt.Sprintf("%s:all:bits:average", scope)
	allBitsOfInfoStd := fmt.Sprintf("%s:all:bits:std", scope)

	db.client.Set(ctx, allBitsOfInfoAverage, mean, 0)
	db.client.Set(ctx, allBitsOfInfoStd, std, 0)

	return mean, std
}

func getAverageStd(ctx *gin.Context, db *database, scope string) (float64, float64) {
	allBitsOfInfoAverage := fmt.Sprintf("%s:all:bits:average", scope)
	allBitsOfInfoStd := fmt.Sprintf("%s:all:bits:std", scope)

	mean, _ := db.client.Get(ctx, allBitsOfInfoAverage).Float64()
	if math.IsNaN(mean) || mean == 0 {
		return 0.0, 1.0
	}

	std, _ := db.client.Get(ctx, allBitsOfInfoStd).Float64()
	if math.IsNaN(std) {
		return mean, 1.0
	}

	return mean, std
}

func calculateAverageStdForAction(ctx *gin.Context, db *database, scope string, action string, id string) (float64, float64) {
	const SampleSize = 1200

	var values []float64
	var sum float64
	var mean, std float64

	key := fmt.Sprintf("%s:single:action:%s:counters:distinct", scope, action)

	kv, err := db.client.HRandFieldWithValues(ctx, key, SampleSize).Result()
	if err != nil {
		panic(err)
	}
	if len(kv) == 0 {
		return mean, 1
	}

	for k := range kv {
		if kv[k].Value != "NaN" && kv[k].Key != id {
			f, err := strconv.ParseFloat(kv[k].Value, 32)
			if err != nil {
				fmt.Println("Can't convert v to int64", kv[k].Value)
				continue
			}
			values = append(values, f)
			sum += f
		}
	}

	if len(values) > 0 {
		mean = sum / float64(len(values))
	}

	for _, value := range values {
		std += math.Pow(value-mean, 2)
	}

	std = math.Sqrt(std / float64(len(values)))

	actionBitsOfInfoAverage := fmt.Sprintf("%s:action:%s:bits:average", scope, action)
	actionBitsOfInfoStd := fmt.Sprintf("%s:action:%s:bits:std", scope, action)

	db.client.Set(ctx, actionBitsOfInfoAverage, mean, 0)
	db.client.Set(ctx, actionBitsOfInfoStd, std, 0)

	return mean, std
}

func getAverageStdForAction(ctx *gin.Context, db *database, scope string, action string) (float64, float64) {
	actionBitsOfInfoAverage := fmt.Sprintf("%s:action:%s:bits:average", scope, action)
	actionBitsOfInfoStd := fmt.Sprintf("%s:action:%s:bits:std", scope, action)

	mean, _ := db.client.Get(ctx, actionBitsOfInfoAverage).Float64()
	if math.IsNaN(mean) || mean == 0 {
		return 0.0, 1.0
	}

	std, _ := db.client.Get(ctx, actionBitsOfInfoStd).Float64()
	if math.IsNaN(std) {
		return mean, 1.0
	}

	return mean, std
}

func partialUpdateMean(ctx *gin.Context, db *database, scope string, mean float64, bitsOfInfo float64, oldpop int64, newpop int64) error {
	allBitsOfInfoAverage := fmt.Sprintf("%s:all:bits:average", scope)
	allBitsOfInfoStd := fmt.Sprintf("%s:all:bits:std", scope)

	pipe := db.client.TxPipeline()

	if oldpop == 0 {
		pipe.Set(ctx, allBitsOfInfoAverage, bitsOfInfo, 0)
		pipe.Set(ctx, allBitsOfInfoStd, 0, 0)
	}

	if oldpop == newpop {
		removing := 1.0 / float64(oldpop) * mean
		adding := 1.0 / float64(newpop) * bitsOfInfo
		pipe.Set(ctx, allBitsOfInfoAverage, mean-removing+adding, 0)
	}

	if oldpop < newpop {
		remaining := float64(oldpop) / float64(newpop) * mean
		adding := (float64(newpop-oldpop) / float64(newpop)) * bitsOfInfo
		pipe.Set(ctx, allBitsOfInfoAverage, remaining+adding, 0)
	}

	_, err := pipe.Exec(ctx)

	if err != nil {
		panic(err)
	}

	return nil
}

func partialActionUpdateMean(ctx *gin.Context, db *database, scope string, action string, mean float64, bitsOfInfo float64, oldpop int64, newpop int64) error {
	actionBitsOfInfoAverage := fmt.Sprintf("%s:all:actions:%s:bits:average", scope, action)
	actionBitsOfInfoStd := fmt.Sprintf("%s:all:actions:%s:bits:std", scope, action)

	pipe := db.client.TxPipeline()

	if oldpop == 0 {
		pipe.Set(ctx, actionBitsOfInfoAverage, bitsOfInfo, 0)
		pipe.Set(ctx, actionBitsOfInfoStd, 0, 0)
	}

	if oldpop == newpop {
		removing := 1.0 / float64(oldpop) * mean
		adding := 1.0 / float64(newpop) * bitsOfInfo
		pipe.Set(ctx, actionBitsOfInfoAverage, mean-removing+adding, 0)
	}

	if oldpop < newpop {
		remaining := float64(oldpop) / float64(newpop) * mean
		adding := (float64(newpop-oldpop) / float64(newpop)) * bitsOfInfo
		pipe.Set(ctx, actionBitsOfInfoStd, remaining+adding, 0)
	}

	_, err := pipe.Exec(ctx)

	if err != nil {
		panic(err)
	}

	return nil
}

func newDatabase(ctx context.Context, address, secretName string) (*database, error) {
	if len(address) == 0 {
		return nil, errors.New("no redis server provided")
	}

	// create redis client
	redisOptions, err := redis.ParseURL(address)

	if err != nil {
		fmt.Println("Failed to parse redis url", err)
	}

	client := redis.NewClient(redisOptions)

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	status, err := client.Ping(context.Background()).Result()

	if err != nil {
		fmt.Println("Failed to connect to redis", err)

	}
	fmt.Printf("Connected to redis cache: %s\n", status)

	return &database{
		client: client,
	}, nil
}

func calculateZScore(value, mean, std float64) float64 {
	if std == 0 {
		return 0
	}
	result := (value - mean) / std
	if math.IsNaN(result) {
		return 0.0
	}
	return result
}
