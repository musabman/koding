package main

import (
	"encoding/json"
	"fmt"
	"github.com/streadway/amqp"
	"koding/databases/neo4j"
	"koding/tools/amqputil"
	"koding/workers/neo4jfeeder/mongohelper"
	"labix.org/v2/mgo/bson"
	"strings"
	"time"
)

var (
	EXCHANGE_NAME     = "graphFeederExchange"
	WORKER_QUEUE_NAME = "graphFeederWorkerQueue"
	TIME_FORMAT       = "2006-01-02T15:04:05.000Z"
)

type Consumer struct {
	conn    *amqp.Connection
	channel *amqp.Channel
}

type Message struct {
	Event   string                   `json:"event"`
	Payload []map[string]interface{} `json:"payload"`
}

func main() {
	startConsuming()
}

//here, mapping of decoded json
func jsonDecode(data string) (*Message, error) {
	source := &Message{}
	err := json.Unmarshal([]byte(data), &source)
	if err != nil {
		return source, err
	}

	return source, nil
}

func startConsuming() {

	c := &Consumer{
		conn:    nil,
		channel: nil,
	}

	c.conn = amqputil.CreateConnection("neo4jFeeding")
	c.channel = amqputil.CreateChannel(c.conn)

	err := c.channel.ExchangeDeclare(EXCHANGE_NAME, "fanout", true, false, false, false, nil)
	if err != nil {
		fmt.Println("exchange.declare: %s", err)
		panic(err)
	}

	//name, durable, autoDelete, exclusive, noWait, args Table
	if _, err := c.channel.QueueDeclare(WORKER_QUEUE_NAME, true, false, false, false, nil); err != nil {
		fmt.Println("queue.declare: %s", err)
		panic(err)
	}

	if err := c.channel.QueueBind(WORKER_QUEUE_NAME, "" /* binding key */, EXCHANGE_NAME, false, nil); err != nil {
		fmt.Println("queue.bind: %s", err)
		panic(err)
	}

	//(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args Table) (<-chan Delivery, error) {
	relationshipEvent, err := c.channel.Consume(WORKER_QUEUE_NAME, "neo4jFeeding", false, false, false, false, nil)
	if err != nil {
		fmt.Println("basic.consume: %s", err)
		panic(err)
	}

	fmt.Println("Neo4J Feeder worker started")

	for msg := range relationshipEvent {
		body := fmt.Sprintf("%s", msg.Body)

		message, err := jsonDecode(body)
		if err != nil {
			fmt.Println("Wrong message format", err, body)

			continue
		}

		if len(message.Payload) < 1 {
			fmt.Println("Wrong message format; payload should be an Array", message)

			continue
		}
		data := message.Payload[0]

		fmt.Println(message.Event)
		if message.Event == "RelationshipSaved" {
			createNode(data)
		} else if message.Event == "RelationshipRemoved" {
			deleteRelationship(data)
		} else if message.Event == "updateInstance" {
			updateNode(data)
		} else if message.Event == "RemovedFromCollection" {
			deleteNode(data)
		} else {
			fmt.Println(message.Event)
		}

		msg.Ack(true)
	}
}

func checkIfEligible(sourceName, targetName string) bool {
	notAllowedSuffixes := []string{
		"Bucket",
		"BucketActivity",
	}

	for _, name := range neo4j.NotAllowedNames {
		if name == sourceName {
			fmt.Println("not eligible " + sourceName)
			return false
		}

		if name == targetName {
			fmt.Println("not eligible " + targetName)
			return false
		}
	}

	for _, name := range notAllowedSuffixes {

		if strings.HasSuffix(sourceName, name) {
			fmt.Println("not eligible " + sourceName)
			return false
		}

		if strings.HasSuffix(targetName, name) {
			fmt.Println("not eligible " + targetName)
			return false
		}

	}

	return true
}

func createNode(data map[string]interface{}) {

	sourceId := fmt.Sprintf("%s", data["sourceId"])
	sourceName := fmt.Sprintf("%s", data["sourceName"])

	targetId := fmt.Sprintf("%s", data["targetId"])
	targetName := fmt.Sprintf("%s", data["targetName"])

	if !checkIfEligible(sourceName, targetName) {
		return
	}

	sourceContent, err := mongohelper.FetchContent(bson.ObjectIdHex(sourceId), sourceName)
	if err != nil {
		fmt.Println("sourceContent", err)
		return
	}
	sourceNode := neo4j.CreateUniqueNode(sourceId, sourceName)
	neo4j.UpdateNode(sourceId, sourceContent)

	targetContent, err := mongohelper.FetchContent(bson.ObjectIdHex(targetId), targetName)
	if err != nil {
		fmt.Println("targetContent", err)
		return
	}
	targetNode := neo4j.CreateUniqueNode(targetId, targetName)
	neo4j.UpdateNode(targetId, targetContent)

	source := fmt.Sprintf("%s", sourceNode["create_relationship"])
	target := fmt.Sprintf("%s", targetNode["self"])

	if _, ok := data["as"]; !ok {
		fmt.Println("as value is not set on this relationship. Discarding this record", data)
		return
	}
	as := fmt.Sprintf("%s", data["as"])

	if _, ok := data["_id"]; !ok {
		fmt.Println("id value is not set on this relationship. Discarding this record", data)
		return
	}

	createdAt := getCreatedAtDate(data)
	relationshipData := fmt.Sprintf(`{"createdAt" : "%s", "createdAtEpoch" : %d }`, createdAt.Format(TIME_FORMAT), createdAt.Unix())
	neo4j.CreateRelationshipWithData(as, source, target, relationshipData)
}

func getCreatedAtDate(data map[string]interface{}) time.Time {

	if _, ok := data["timestamp"]; ok {
		t, err := time.Parse(TIME_FORMAT, data["timestamp"].(string))
		// if error doesnt exists, return createdAt
		if err == nil {
			return t.UTC()
		}
	}

	id := fmt.Sprintf("%s", data["_id"])
	if bson.IsObjectIdHex(id) {
		return bson.ObjectIdHex(id).Time().UTC()
	}

	fmt.Print("Couldnt determine the createdAt time, returning Now() as creatdAt")
	return time.Now().UTC()
}

func deleteNode(data map[string]interface{}) {

	if _, ok := data["_id"]; !ok {
		return
	}
	id := fmt.Sprintf("%s", data["_id"])
	neo4j.DeleteNode(id)
}

func deleteRelationship(data map[string]interface{}) {
	sourceId := fmt.Sprintf("%s", data["sourceId"])
	targetId := fmt.Sprintf("%s", data["targetId"])
	as := fmt.Sprintf("%s", data["as"])

	// we are not doing anything with result for now
	// do not pollute console
	neo4j.DeleteRelationship(sourceId, targetId, as)
	//result := neo4j.DeleteRelationship(sourceId, targetId, as)
	//if result {
	//	fmt.Println("Relationship deleted")
	//} else {
	//	fmt.Println("Relationship couldnt be deleted")
	//}
}

func updateNode(data map[string]interface{}) {

	if _, ok := data["bongo_"]; !ok {
		return
	}
	if _, ok := data["data"]; !ok {
		return
	}

	bongo := data["bongo_"].(map[string]interface{})
	obj := data["data"].(map[string]interface{})

	sourceId := fmt.Sprintf("%s", obj["_id"])
	sourceName := fmt.Sprintf("%s", bongo["constructorName"])

	if !checkIfEligible(sourceName, "") {
		return
	}

	sourceContent, err := mongohelper.FetchContent(bson.ObjectIdHex(sourceId), sourceName)
	if err != nil {
		fmt.Println("sourceContent", err)
		return
	}

	neo4j.CreateUniqueNode(sourceId, sourceName)
	neo4j.UpdateNode(sourceId, sourceContent)
}
