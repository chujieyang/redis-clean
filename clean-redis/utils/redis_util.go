package utils

import (
	"fmt"
	"github.com/gomodule/redigo/redis"
	"time"
)

var redisPool *redis.Pool
var redisClient redis.Conn

type IterDelRedisKeys interface  {
	del() (err error)
}

type StringKey struct {
	keyName string
}

func (k StringKey) del() (err error) {
	_, err = redisClient.Do("del", k.keyName)
	return
}

type HashKey struct {
	keyName string
}

func (k HashKey) del() (err error) {
	iter := 0
	for {
		arr, err1 := redis.Values(redisClient.Do("HSCAN", k.keyName, iter))
		if err1 != nil {
			fmt.Println(err1)
			err = err1
			return
		}
		iter, _ = redis.Int(arr[0], nil)
		keysList, _ := redis.Strings(arr[1], nil)
		var hashKeys []interface{}
		for index, keyItem := range keysList {  // 迭代删除 hash key
			if index % 2 == 0 {
				hashKeys = append(hashKeys, keyItem)
			}
			if len(hashKeys) == 20 {
				var args []interface{}
				args = append(args, k.keyName)
				args = append(args, hashKeys...)
				if _, err := redisClient.Do("HDEL", args...); err != nil {
					fmt.Println(err)
				} else {
					hashKeys = []interface{}{}
				}
			}
		}
		if iter == 0 {
			var args []interface{}
			args = append(args, k.keyName)
			args = append(args, hashKeys...)
			if _, err := redisClient.Do("HDEL", args...); err != nil {
				fmt.Println(err)
			}
			break
		}
	}
	return
}

type ListKey struct {
	keyName string
}

func (k ListKey) del() (err error) {
	length, err := redis.Int(redisClient.Do("LLEN", k.keyName))
	if err != nil {
		return
	}
	for length > 0 {
		if _, err = redisClient.Do("LTRIM", k.keyName, 0, -20); err != nil {
			fmt.Println(err)
			return
		}
		if length, err = redis.Int(redisClient.Do("LLEN", k.keyName)); err != nil {
			return
		}
	}
	return
}

type SetKey struct {
	keyName string
}

func (k SetKey) del() (err error) {
	iter := 0
	for {
		arr, err1 := redis.Values(redisClient.Do("SSCAN", k.keyName, iter))
		if err1 != nil {
			fmt.Println(err1)
			err = err1
			return
		}
		iter, _ = redis.Int(arr[0], nil)
		keysList, _ := redis.Strings(arr[1], nil)
		var hashKeys []interface{}
		for _, keyItem := range keysList {  // 迭代删除 hash key
			hashKeys = append(hashKeys, keyItem)
			if len(hashKeys) == 20 {
				fmt.Println("批量删除")
				var args []interface{}
				args = append(args, k.keyName)
				args = append(args, hashKeys...)
				if _, err := redisClient.Do("SREM", args...); err != nil {
					fmt.Println(err)
				} else {
					hashKeys = []interface{}{}
				}
			}
		}
		if iter == 0 {
			fmt.Println("查询结束删除")
			var args []interface{}
			args = append(args, k.keyName)
			args = append(args, hashKeys...)
			if _, err := redisClient.Do("SREM", args...); err != nil {
				fmt.Println(err)
			}
			break
		}
	}
	return
}

type ZSetKey struct {
	keyName string
}

func (k ZSetKey) del() (err error) {
	length, err := redis.Int(redisClient.Do("ZCARD", k.keyName))
	if err != nil {
		return
	}
	for length > 0 {
		if _, err = redisClient.Do("ZREMRANGEBYRANK", k.keyName, 0, 10); err != nil {
			fmt.Println(err)
			return
		}
		if length, err = redis.Int(redisClient.Do("ZCARD", k.keyName)); err != nil {
			return
		}
	}
	return
}

func InitRedisPool(connectionString string, db int, auth string) {
	redisPool = &redis.Pool{
		MaxIdle:     256,
		MaxActive:   1,  // 线程池大小
		IdleTimeout: time.Duration(120),
		Wait: true,
		Dial: func() (redis.Conn, error) {
			return redis.Dial(
				"tcp",
				connectionString,
				redis.DialReadTimeout(time.Duration(1000)*time.Millisecond),
				redis.DialWriteTimeout(time.Duration(1000)*time.Millisecond),
				redis.DialConnectTimeout(time.Duration(1000)*time.Millisecond),
				redis.DialDatabase(db),
				redis.DialPassword(auth),
			)
		},
	}
	redisClient = redisPool.Get()
}

func getRedisKeyType(key string) (keyType string, err error) {
	keyType, err = redis.String(redisClient.Do("type", key))
	return
}

func RemoveRedisKeys(pattern string) (err error) {
	iter := 0
	var redisKeyItem IterDelRedisKeys
	var keys []string
	for {
		arr, err := redis.Values(redisClient.Do("SCAN", iter, "MATCH", pattern))
		if err != nil {
			return fmt.Errorf("error retrieving '%s' keys", pattern)
		}
		iter, _ = redis.Int(arr[0], nil)
		k, _ := redis.Strings(arr[1], nil)
		keys = append(keys, k...)
		for _, keyItem := range k {
			if keyType, err := getRedisKeyType(keyItem); err != nil {
				fmt.Println(err)
				break
			} else {
				fmt.Println(fmt.Sprintf("Found Key: %s, type: %s", keyItem, keyType))
				switch keyType {
				case "string":
					redisKeyItem = StringKey{
						keyName: keyItem,
					}
				case "hash":
					redisKeyItem = HashKey{
						keyName: keyItem,
					}
				case "list":
					redisKeyItem = ListKey{
						keyName: keyItem,
					}
				case "set":
					redisKeyItem = SetKey{
						keyName: keyItem,
					}
				case "zset":
					redisKeyItem = ZSetKey{
						keyName: keyItem,
					}
				default:
					fmt.Println("不支持的类型: ", keyType)
					break
				}
				if err = redisKeyItem.del(); err != nil {
					fmt.Println(fmt.Sprintf("Delete Key: %s [Failed]", keyItem))
					break
				} else {
					fmt.Println(fmt.Sprintf("Delete Key: %s [Success]", keyItem))
				}
			}
		}
		if iter == 0 {
			break
		}
	}
	return
}