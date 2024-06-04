package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func pingHandler(c *gin.Context) {
	startTime := time.Now()

	var res AliveResponseSpec

	endTime := time.Now()

	res.Status = "Alive"
	res.Runtime = endTime.Sub(startTime).Milliseconds()

	c.JSON(http.StatusOK, res)

}

func discreteHandler(c *gin.Context, db *database, hub *Hub) {
	startTime := time.Now()

	var e EventSpec

	if err := c.ShouldBindJSON(&e); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(e.Organization) == 0 || len(e.Source) == 0 || len(e.Population) == 0 || len(e.Identifier) == 0 || len(e.Action) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error"})
		return
	}

	scope := fmt.Sprintf("%s:%s:%s", e.Organization, e.Source, e.Population)

	const popRefresh = 4096
	const idRefresh = 64
	const idNormalizeTo = 100

	popRefreshStep := int64(popRefresh)
	idRefreshStep := int64(idRefresh)

	// count the population (using hyperloglog)
	oldPopulationCount := GetPopulationCount(c, db, scope)
	newPopulationCount := UpdatePopulationCount(c, db, scope, e.Identifier)
	populationActionCount, populationTotalActionCount := IncreasePopulationActionCount(c, db, scope, e.Action)
	idActionCount, idTotalActionCount := IncreaseIdActionCount(c, db, scope, e.Identifier, e.Action)

	var bitsOfInfo float64

	if idTotalActionCount < idRefresh {
		idRefreshStep = int64(math.Max(8, math.Min(math.Pow(2, math.Ceil(math.Log2(float64(idTotalActionCount)))), idRefresh)))
	}

	if (idTotalActionCount+1)%idRefreshStep == 0 {
		mapGlobalActionCount := GetGlobalCounts(c, db, scope)
		mapIdActionCount := GetIdCounts(c, db, scope, e.Identifier)

		bitsOfInfo = idCompleteBitsOfInfo(c, db, scope, e.Identifier, mapGlobalActionCount, mapIdActionCount, populationTotalActionCount, idTotalActionCount, float64(idNormalizeTo))

	} else { // partial update
		bitsOfInfo = idActionBitsOfInfo(c, db, scope, e.Identifier, e.Action, populationTotalActionCount, populationActionCount, idTotalActionCount, idActionCount, float64(idNormalizeTo))
	}

	var mean, std float64
	std = 1

	// I want to refresh the mean and std often when the code starts, but then I want it to do it on a less regular basis.
	// This this calculation allows the code to refresh the mean and std at every power of 2, until it reaches a stable sample size (popRefresh == 4096)
	if populationTotalActionCount < popRefresh {
		popRefreshStep = int64(math.Max(8, math.Min(math.Pow(2, math.Ceil(math.Log2(float64(populationTotalActionCount)))), popRefresh)))
	}

	if (populationTotalActionCount+1)%popRefreshStep == 0 {
		mean, std = calculateAverageStd(c, db, scope, e.Identifier)
		// fmt.Println(scope, populationTotalActionCount, popRefreshStep, "mean: ", mean, "std:", std)
	} else {
		mean, std = getAverageStd(c, db, scope)
		partialUpdateMean(c, db, scope, mean, bitsOfInfo, oldPopulationCount, newPopulationCount)
	}

	//fmt.Println("Finished processing event")

	var res DiscreteResponseSpec
	res.Identifier = e.Identifier
	res.Score = roundFloat(bitsOfInfo, 3)
	res.Count = idTotalActionCount
	res.Zscore = 0.0

	if std != 0 {
		res.Zscore = roundFloat((bitsOfInfo-mean)/std, 3)
	}

	res.Source = e.Source
	res.Population = e.Population
	res.Timestamp = e.Timestamp
	res.Action = e.Action
	endTime := time.Now()
	res.Runtime = endTime.Sub(startTime).Milliseconds()

	d, err := json.Marshal(res)
	if err != nil {
		fmt.Println("FATAL ERROR: marshaling output:", err)
	}

	payload := map[string][]byte{
		"message": d,
		"id":      []byte("server"),
	}

	serverMessage, err := json.Marshal(payload)
	if err != nil {
		fmt.Println("FATAL ERROR: marshaling output:", err)

	}

	hub.broadcast <- serverMessage

	c.JSON(http.StatusOK, res)
}

func roundFloat(val float64, precision uint) float64 {
	ratio := math.Pow(10, float64(precision))
	return math.Round(val*ratio) / ratio
}
