package mongodb

import (
	"context"
	"time"

	"github.com/globalsign/mgo"

	"github.com/philippgille/gokv/encoding"
	"github.com/philippgille/gokv/util"
)

// item is the document that's stored in the MongoDB collection.
// mgo (un-)marshalls it to/from bson automatically, when reading from / writing to MongoDB.
// It sounds like (un-)marshalling twice is inefficient, but the mgo (un-)marshalling
// only allows maps and structs.
// Having the gokv package user's value marshalled by ourselves allows any value to be used,
// so the MongoDB implementation works the same as any other gokv.Store implementation.
// See https://github.com/globalsign/mgo/blob/113d3961e7311526535a1ef7042196563d442761/bson/bson.go#L538.
type item struct {
	// There are advantages and disavantages regarding the use of a string as "_id" instead of MongoDB's default ObjectId.
	// We can't use the ObjectId because we only have the key that the gokv package user passes us as parameter.
	// We could use a document with "_id" as ObjectId, "k" as string and "v" as slice of bytes and then create an MongoDB Index for "k".
	// That would have the advantage that we could activate the constraint that the indexed values must be unique
	// (which is not the case with the "_id" field, which is rarely realized due to the use of ObjectId as "_id"
	// and ObjectId being generated on the server to guarantee uniqueness).
	// But it would have the disadvantage that when clustering the MongoDB and sharding the MongoDB collection that we use,
	// the DB admin would have to use *our* indexed value as shard key, because otherwise it could lead to duplicate entries
	// even if the unique constraint is set. And the admin might not be aware of this. Using "_id" as shard key seems to be pretty standard.
	// At least that (advantages + disadvantages) is my understanding from the documentation and comments on Stackoverflow.
	// Relevant links:
	// - https://github.com/mongodb/docs/blob/5f2d5e7dce7766a14b25b0d032970f065a866110/source/core/document.txt#L78
	// - https://github.com/mongodb/docs/blob/e1b05bac8616fdfac13bedd79516a5ac33d4afdf/source/reference/bson-types.txt#L41
	// - https://github.com/mongodb/docs/blob/85171fd9fcc1cf2a5dc6f297b2b026c86bfbfd9d/source/indexes.txt#L46
	// - https://github.com/mongodb/docs/blob/81d03d2463bc995a451759ce44087fe7ecd4db74/source/core/sharding-shard-key.txt#L91
	//
	// There are multiple ways to tag for mgo: https://github.com/globalsign/mgo/blob/113d3961e7311526535a1ef7042196563d442761/bson/bson.go#L538.
	// But without "bson" go_vet says: "struct field tag `_id` not compatible with reflect.StructTag.Get: bad syntax for struct tag pair"
	K string `bson:"_id"`
	V []byte // "v" will be used as field name
}

// Client is a gokv.Store implementation for MongoDB.
type Client struct {
	c *mgo.Collection
	// Only needed for closing.
	session *mgo.Session
	codec   encoding.Codec
}

// Set stores the given value for the given key.
// Values are automatically marshalled to JSON or gob (depending on the configuration).
// The key must not be "" and the value must not be nil.
func (c Client) Set(k string, v any) error {
	ctx := context.Background()
	return c.SetWithContext(ctx, k, v)
}

// SetWithContext is exactly like Set function just with added context as first argument.
func (c Client) SetWithContext(_ context.Context, k string, v any) error {
	if err := util.CheckKeyAndValue(k, v); err != nil {
		return err
	}

	// First turn the passed object into something that MongoDB can handle
	data, err := c.codec.Marshal(v)
	if err != nil {
		return err
	}

	item := item{
		// K needs to be specified, otherwise an update operation (on an existing document) would lead to the "_id" being overwritten by "",
		// which 1) we don't want of course and 2) leads to an error anyway.
		K: k,
		V: data,
	}
	_, err = c.c.UpsertId(k, item)
	if err != nil {
		return err
	}

	return nil
}

// Get retrieves the stored value for the given key.
// You need to pass a pointer to the value, so in case of a struct
// the automatic unmarshalling can populate the fields of the object
// that v points to with the values of the retrieved object's values.
// If no value is found it returns (false, nil).
// The key must not be "" and the pointer must not be nil.
func (c Client) Get(k string, v any) (found bool, err error) {
	ctx := context.Background()
	return c.GetWithContext(ctx, k, v)
}

// GetWithContext is exactly like Get function just with added context as first argument.
func (c Client) GetWithContext(_ context.Context, k string, v any) (found bool, err error) {
	if err := util.CheckKeyAndValue(k, v); err != nil {
		return false, err
	}

	item := new(item)
	err = c.c.FindId(k).One(item)
	// If no value was found return false
	if err == mgo.ErrNotFound {
		return false, nil
	} else if err != nil {
		return false, err
	}
	data := item.V

	return true, c.codec.Unmarshal(data, v)
}

// Delete deletes the stored value for the given key.
// Deleting a non-existing key-value pair does NOT lead to an error.
// The key must not be "".
func (c Client) Delete(k string) error {
	ctx := context.Background()
	return c.DeleteWithContext(ctx, k)
}

// DeleteWithContext is exactly like Delete function just with added context as first argument.
func (c Client) DeleteWithContext(_ context.Context, k string) error {
	if err := util.CheckKey(k); err != nil {
		return err
	}

	err := c.c.RemoveId(k)
	if err != mgo.ErrNotFound {
		return err
	}
	return nil
}

// Close closes the client.
// It must be called to release any open resources.
func (c Client) Close() error {
	c.session.Close()
	return nil
}

// Options are the options for the MongoDB client.
type Options struct {
	// Seed servers for the initial connection to the MongoDB cluster.
	// Format: [mongodb://][user:pass@]host1[:port1][,host2[:port2],...][/database][?options].
	// E.g.: "localhost" (the port defaults to 27017).
	// Optional ("localhost" by default).
	// For a detailed documentation and more examples see https://github.com/mongodb/docs/blob/01fa14decadc116b09ecdeae049e6744f16bf97f/source/reference/connection-string.txt.
	// For the options you need to stick to the mgo documentation (the package that gokv uses) instead of the official MongoDB documentation:
	// https://github.com/globalsign/mgo/blob/113d3961e7311526535a1ef7042196563d442761/session.go#L236.
	ConnectionString string
	// The name of the database to use.
	// Optional ("gokv" by default).
	DatabaseName string
	// The name of the collection to use.
	// Optional ("item" by default).
	CollectionName string
	// Encoding format.
	// Optional (encoding.JSON by default).
	Codec encoding.Codec
}

// DefaultOptions is an Options object with default values.
// ConnectionString: "localhost", DatabaseName: "gokv", CollectionName: "item", Codec: encoding.JSON
var DefaultOptions = Options{
	ConnectionString: "localhost",
	DatabaseName:     "gokv",
	CollectionName:   "item",
	Codec:            encoding.JSON,
}

// NewClient creates a new MongoDB client.
//
// You must call the Close() method on the client when you're done working with it.
func NewClient(options Options) (Client, error) {
	result := Client{}

	// Set default values
	if options.ConnectionString == "" {
		options.ConnectionString = DefaultOptions.ConnectionString
	}
	if options.DatabaseName == "" {
		options.DatabaseName = DefaultOptions.DatabaseName
	}
	if options.CollectionName == "" {
		options.CollectionName = DefaultOptions.CollectionName
	}
	if options.Codec == nil {
		options.Codec = DefaultOptions.Codec
	}

	session, err := mgo.DialWithTimeout(options.ConnectionString, 2*time.Second)
	if err != nil {
		return result, err
	}
	c := session.DB(options.DatabaseName).C(options.CollectionName)

	result.c = c
	result.session = session
	result.codec = options.Codec

	return result, nil
}
