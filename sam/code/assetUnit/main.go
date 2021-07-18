package main

import (
	"encoding/json"
	"math"
	"os"
	"reflect"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/guregu/dynamo"
)

type AssetUnit struct {
	AssetCode string
	Date      string
	Unit      int
	Amount    int
}

type AssetUnitReq struct {
	AssetCode string `json:"AssetCode"`
	Date      string `json:"Date"`
	Unit      int    `json:"Unit"`
	Amount    int    `json:"Amount"`
}

type AssetDaily struct {
	AssetCode string
	Date      string
	Price     int
}

func main() {
	lambda.Start(handler)
}

// メインハンドラー
func handler(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	var (
		assetUnitData []AssetUnit
		err           error
	)

	// リクエストがPOSTかGETで実行する処理を分岐する
	switch request.HTTPMethod {
	case "POST":
		// リクエストボディ取得
		reqBody := request.Body
		jsonBytes := ([]byte)(reqBody)
		assetUnitReq := new(AssetUnitReq)
		if err := json.Unmarshal(jsonBytes, assetUnitReq); err != nil {
			return events.APIGatewayProxyResponse{}, err
		}
		assetUnitData, err = postHandler(assetUnitReq)
	case "GET":
		// パス・クエリパラメータ取得
		assetCode := request.PathParameters["assetCode"]
		date := request.QueryStringParameters["date"]
		assetUnitData, err = getHandler(assetCode, date)
	}
	if err != nil {
		return events.APIGatewayProxyResponse{}, err
	}
	// AssetCode毎にリストを格納
	assetUnitDataByAssetCode := make(map[string][]AssetUnit)
	for _, data := range assetUnitData {
		assetUnitDataByAssetCode[data.AssetCode] = append(assetUnitDataByAssetCode[data.AssetCode], data)
	}

	jsonBytes, _ := json.Marshal(assetUnitDataByAssetCode)
	return events.APIGatewayProxyResponse{
		Headers: map[string]string{
			"Access-Control-Allow-Origin":      os.Getenv("ALLOW_ORIGIN"),
			"Access-Control-Allow-Headers":     "X-Requested-With, Origin, X-Csrftoken, Content-Type, Accept",
			"Access-Control-Allow-Credentials": "true",
			"Content-Type":                     "application/json",
		},
		Body:       string(jsonBytes),
		StatusCode: 200,
	}, nil
}

// データ取得
func getHandler(assetCode string, date string) ([]AssetUnit, error) {
	var assetUnit []AssetUnit
	// Dynamodb接続
	table := connectDynamodb("asset_unit")
	filter := table.Scan()
	// 取得条件設定
	if assetCode != "" {
		filter = filter.Filter("'AssetCode' = ?", assetCode)
	}
	if date != "" {
		filter = filter.Filter("'Date' = ?", date)
	}
	err := filter.All(&assetUnit)
	return assetUnit, err
}

// データ登録
func postHandler(assetUnitReq *AssetUnitReq) ([]AssetUnit, error) {
	assetCode := assetUnitReq.AssetCode
	date := assetUnitReq.Date
	amount := float64(assetUnitReq.Amount)
	unit := float64(assetUnitReq.Unit)

	// 対象日の基準価格を取得
	price, _ := getPrice(assetCode, date)
	// 金額を引数に口数を計算する
	if amount != 0 {
		unit = math.Round(float64(amount) / float64(price) * 10000)
	}
	// 口数を引数に金額を計算する
	if unit != 0 {
		amount = math.Round(float64(price) * float64(unit) / 10000)
	}

	assetAmount := []AssetUnit{
		{AssetCode: assetCode, Date: date, Unit: int(unit), Amount: int(amount)},
	}
	// Dynamodb接続
	table := connectDynamodb("asset_unit")
	// 資産データ登録
	err := table.Put(assetAmount).Run()

	return assetAmount, err
}

func getPrice(assetCode string, date string) (int, error) {
	var assetDailyData AssetDaily
	// Dynamodb接続
	table := connectDynamodb("asset_daily")
	// 資産価値データ取得
	if assetCode == "" && date == "" {
		return 0, nil
	}
	err := table.Get("AssetCode", assetCode).Range("Date", dynamo.Equal, date).One(&assetDailyData)
	price := assetDailyData.Price

	return price, err
}

// Dynamodb接続設定
func connectDynamodb(table string) dynamo.Table {
	// Endpoint設定(Local Dynamodb接続用)
	endpoint := os.Getenv("DYNAMODB_ENDPOINT")
	// Dynamodb接続設定
	session := session.Must(session.NewSession())
	config := aws.NewConfig().WithRegion("ap-northeast-1")
	if len(endpoint) > 0 {
		config = config.WithEndpoint(endpoint)
	}
	db := dynamo.New(session, config)
	return db.Table(table)
}

// 構造体を連想配列に変換
func structToMap(data interface{}) []map[string]interface{} {
	valueList := reflect.ValueOf(data)
	arraySize := valueList.Len()
	mapDataList := make([]map[string]interface{}, 0, arraySize)

	for arrayIndex := 0; arrayIndex < arraySize; arrayIndex++ {
		data := valueList.Index(arrayIndex)
		dataType := data.Type()
		// マップに代入
		mapData := make(map[string]interface{})
		for mapIndex := 0; mapIndex < dataType.NumField(); mapIndex++ {
			mapData[dataType.Field(mapIndex).Name] = data.Field(mapIndex).Interface()
		}
		mapDataList = append(mapDataList, mapData)
	}
	return mapDataList
}
