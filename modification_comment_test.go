// To check if comment is kept after modification

package yaml_test

import (
	"bytes"
	"fmt"
	"log"
	"strconv"

	yaml "github.com/Pixl-SG/yaml"
	"github.com/pkg/errors"
	. "gopkg.in/check.v1"
)

var (
	sourceYaml = `version: 2
jobs:
- schedule: 0 0/5 * 1/1 * ? *
  type: shits
  config:
    host: mongodb://localhost:27017/admin?replicaSet=rs
    minSecondaries: 2
    minOplogHours: 100
    maxSecondaryDelay: 120
# shit here
- name: B
  schedule: 0 0/5 * 1/1 * ? *
  type: mongodb.cluster
  config:
    host: mongodb://localhost:27017/admin?replicaSet=rs
    minSecondaries: 2
    minOplogHours: 100
    maxSecondaryDelay: 120`

	sourceYamlWithoutComment = `aa: 0
bb: string1
cc: string1
lists:
- list1_key1: value1
  list1_key4:
    list1_sub1: value4
`
	sourceYamlWithoutCommentExpected = `aa: 0
bb: bbstring
cc: string1
lists:
- list1_key1: value1
  list1_key4:
    list1_sub1: value4
`
	sourceYamlWithCommentEnglish = `aa: 0
# single value
bb: string1
cc: string1
# Here is list
lists:
- list1_key1: value1
  list1_key4:
    list1_sub1: value4
`
	sourceYamlWithCommentEnglishExpected = `aa: 0
# single value
bb: bbstring
cc: string1
# Here is list
lists:
- list1_key1: value1
  list1_key4:
    list1_sub1: value4
`
	sourceYamlWithCommentChinese = `aa: 0
# 单个值
bb: string1
cc: string1
# 这里是列表
lists:
- list1_key1: value1
  list1_key4:
    list1_sub2: value5
`

	sourceYamlWithCommentChineseExpected = `aa: 0
# 单个值
bb: bbstring
cc: string1
# 这里是列表
lists:
- list1_key1: value1
  list1_key4:
    list1_sub2: value5
`
)

var enableLog = false

func mylog(format string, values ...interface{}) {
	if !enableLog {
		return
	}
	fmt.Printf(format, values...)
}

func parsePath(path string) []string {
	return parsePathAccum([]string{}, path)
}

func parsePathAccum(paths []string, remaining string) []string {
	head, tail := nextYamlPath(remaining)
	if tail == "" {
		return append(paths, head)
	}
	return parsePathAccum(append(paths, head), tail)
}

func nextYamlPath(path string) (pathElement string, remaining string) {
	switch path[0] {
	case '[':
		// e.g [0].blah.cat -> we need to return "0" and "blah.cat"
		return search(path[1:], []uint8{']'}, true)
	case '"':
		// e.g "a.b".blah.cat -> we need to return "a.b" and "blah.cat"
		return search(path[1:], []uint8{'"'}, true)
	default:
		// e.g "a.blah.cat" -> return "a" and "blah.cat"
		return search(path[0:], []uint8{'.', '['}, false)
	}
}

func search(path string, matchingChars []uint8, skipNext bool) (pathElement string, remaining string) {
	for i := 0; i < len(path); i++ {
		var char = path[i]
		if contains(matchingChars, char) {
			var remainingStart = i + 1
			if skipNext {
				remainingStart = remainingStart + 1
			} else if !skipNext && char != '.' {
				remainingStart = i
			}
			if remainingStart > len(path) {
				remainingStart = len(path)
			}
			return path[0:i], path[remainingStart:]
		}
	}
	return path, ""
}

func contains(matchingChars []uint8, candidate uint8) bool {
	for _, a := range matchingChars {
		if a == candidate {
			return true
		}
	}
	return false
}

func entryInSlice(context yaml.MapSlice, key interface{}) *yaml.MapItem {
	for idx := range context {
		var entry = &context[idx]
		if entry.Key == key {
			return entry
		}
	}
	return nil
}

func getMapSlice(context interface{}) yaml.MapSlice {
	var mapSlice yaml.MapSlice
	switch context.(type) {
	case yaml.MapSlice:
		mapSlice = context.(yaml.MapSlice)
	default:
		mapSlice = make(yaml.MapSlice, 0)
	}
	return mapSlice
}

func getArray(context interface{}) (array []interface{}, ok bool) {
	switch context.(type) {
	case []interface{}:
		array = context.([]interface{})
		ok = true
	default:
		array = make([]interface{}, 0)
		ok = false
	}
	return
}

func writeMap(context interface{}, paths []string, value interface{}) yaml.MapSlice {
	mylog("writeMap for %v for %v with value %v\n", paths, context, value)

	mapSlice := getMapSlice(context)

	if len(paths) == 0 {
		return mapSlice
	}

	child := entryInSlice(mapSlice, paths[0])
	if child == nil {
		newChild := yaml.MapItem{Key: paths[0]}
		mapSlice = append(mapSlice, newChild)
		child = entryInSlice(mapSlice, paths[0])
		mylog("\tAppended child at %v for mapSlice %v\n", paths[0], mapSlice)
	}

	mylog("\tchild.Value %v\n", child.Value)

	remainingPaths := paths[1:]
	child.Value = updatedChildValue(child.Value, remainingPaths, value)
	mylog("\tReturning mapSlice %v\n", mapSlice)
	return mapSlice
}

func updatedChildValue(child interface{}, remainingPaths []string, value interface{}) interface{} {
	if len(remainingPaths) == 0 {
		return value
	}

	_, nextIndexErr := strconv.ParseInt(remainingPaths[0], 10, 64)
	if nextIndexErr != nil && remainingPaths[0] != "+" {
		// must be a map
		return writeMap(child, remainingPaths, value)
	}

	// must be an array
	return writeArray(child, remainingPaths, value)
}

func writeArray(context interface{}, paths []string, value interface{}) []interface{} {
	mylog("writeArray for %v for %v with value %v\n", paths, context, value)
	array, _ := getArray(context)

	if len(paths) == 0 {
		return array
	}

	mylog("\tarray %v\n", array)

	rawIndex := paths[0]
	var index int64
	// the append array indicator
	if rawIndex == "+" {
		index = int64(len(array))
	} else {
		index, _ = strconv.ParseInt(rawIndex, 10, 64)
		index = getRealIndex(array, index)
	}
	for index >= int64(len(array)) {
		array = append(array, nil)
	}
	currentChild := array[index]

	mylog("\tcurrentChild %v\n", currentChild)

	remainingPaths := paths[1:]
	array[index] = updatedChildValue(currentChild, remainingPaths, value)
	mylog("\tReturning array %v\n", array)
	return array
}

func readMap(context yaml.MapSlice, head string, tail []string) (interface{}, error) {
	if head == "*" {
		return readMapSplat(context, tail)
	}
	var value interface{}

	entry := entryInSlice(context, head)
	if entry != nil {
		value = entry.Value
	}
	return calculateValue(value, tail)
}

func readMapSplat(context yaml.MapSlice, tail []string) (interface{}, error) {
	var newArray = make([]interface{}, len(context))
	var i = 0
	for _, entry := range context {
		if len(tail) > 0 {
			val, err := recurse(entry.Value, tail[0], tail[1:])
			if err != nil {
				return nil, err
			}
			newArray[i] = val
		} else {
			newArray[i] = entry.Value
		}
		i++
	}
	return newArray, nil
}

func recurse(value interface{}, head string, tail []string) (interface{}, error) {
	switch value.(type) {
	case []interface{}:
		if head == "*" {
			return readArraySplat(value.([]interface{}), tail)
		}
		index, err := strconv.ParseInt(head, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("Error accessing array: %v", err)
		}
		return readArray(value.([]interface{}), index, tail)
	case yaml.MapSlice:
		return readMap(value.(yaml.MapSlice), head, tail)
	default:
		return nil, nil
	}
}

func getRealIndex(array []interface{}, index int64) int64 {
	for i, value := range array {
		switch value.(type) {
		case yaml.Comment:
			index++
		}
		if int64(i) == index {
			break
		}
	}
	return index
}

func readArray(array []interface{}, head int64, tail []string) (interface{}, error) {
	head = getRealIndex(array, head)
	if head >= int64(len(array)) {
		return nil, nil
	}
	value := array[head]

	return calculateValue(value, tail)
}

func readArraySplat(array []interface{}, tail []string) (interface{}, error) {
	var newArray = make([]interface{}, len(array))
	for index, value := range array {
		val, err := calculateValue(value, tail)
		if err != nil {
			return nil, err
		}
		newArray[index] = val
	}
	return newArray, nil
}

func calculateValue(value interface{}, tail []string) (interface{}, error) {
	if len(tail) > 0 {
		return recurse(value, tail[0], tail[1:])
	}
	return value, nil
}

var docIndex = "0"

type updateDataFn func(dataBucket interface{}, currentIndex int) (interface{}, error)

func modifyKeyValue(source string, key string, value string) (str string, err error) {

	var updateData = func(dataBucket interface{}, currentIndex int) (interface{}, error) {
		docIndexInt, err := strconv.Atoi(docIndex)
		if err != nil {
			return nil, err
		}
		if currentIndex == docIndexInt {
			mylog("Updating index %v\n", currentIndex)
			mylog("setting %v to %v\n", key, value)
			var paths = parsePath(key)
			dataBucket = updatedChildValue(dataBucket, paths, value)
		}

		return dataBucket, nil
	}
	decoded := yaml.MapSlice{}
	err = yaml.Unmarshal([]byte(source), &decoded)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	buf := new(bytes.Buffer)
	encoder := yaml.NewEncoder(buf)
	update(updateData, encoder, decoded)

	return buf.String(), nil
}

type yamlDecoderFn func(*yaml.Decoder) error

func update(updateData updateDataFn, encoder *yaml.Encoder, decoded yaml.MapSlice) error {
	var dataBucket interface{}
	var errorWriting error
	var errorUpdating error
	var currentIndex = 0

	dataBucket = decoded

	dataBucket, errorUpdating = updateData(dataBucket, currentIndex)
	if errorUpdating != nil {
		return errors.Wrapf(errorUpdating, "Error updating document at index %v", currentIndex)
	}

	errorWriting = encoder.Encode(dataBucket)

	if errorWriting != nil {
		return errors.Wrapf(errorWriting, "Error writing document at index %v, %v", currentIndex, errorWriting)
	}

	return nil
}

// Modificaiton without comment
func (s *S) TestModifyWithoutComment(c *C) {
	res, err := modifyKeyValue(sourceYamlWithoutComment, "bb", "bbstring")
	c.Assert(err, IsNil)
	c.Assert(string(res), Equals, string(sourceYamlWithoutCommentExpected))
}

// Modification with comment in ascii, like English
func (s *S) TestModifyWithCommentEnglish(c *C) {
	yaml.DefaultCommentsEnable = true
	res, err := modifyKeyValue(sourceYamlWithCommentEnglish, "bb", "bbstring")
	c.Assert(err, IsNil)
	c.Assert(string(res), Equals, string(sourceYamlWithCommentEnglishExpected))
}

// Modification with comment in utf-8, like Chinese
func (s *S) TestModifyWithCommentChinese(c *C) {
	yaml.DefaultCommentsEnable = true
	res, err := modifyKeyValue(sourceYamlWithCommentChinese, "bb", "bbstring")
	c.Assert(err, IsNil)
	c.Assert(string(res), Equals, string(sourceYamlWithCommentChineseExpected))
}
