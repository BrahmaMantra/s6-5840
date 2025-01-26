package main

//
// 一个用于 MapReduce 的词频统计应用 "插件"。
//
// go build -buildmode=plugin wc.go
//

import (
	"strconv"
	"strings"
	"unicode"

	"6.5840/mr"
)

// Map 函数会对每个输入文件调用一次。第一个参数是输入文件的名称，
// 第二个参数是文件的完整内容。你应该忽略输入文件名，只关注内容参数。
// 返回值是一个键/值对的切片。
func Map(filename string, contents string) []mr.KeyValue {
	// 用于检测单词分隔符的函数。
	ff := func(r rune) bool { return !unicode.IsLetter(r) }

	// 将内容拆分成单词数组。
	words := strings.FieldsFunc(contents, ff)

	kva := []mr.KeyValue{}
	for _, w := range words {
		kv := mr.KeyValue{w, "1"}
		kva = append(kva, kv)
	}
	return kva
}

// Reduce 函数会对每个由 Map 任务生成的键调用一次，
// 并传入由任何 Map 任务为该键创建的所有值的列表。
func Reduce(key string, values []string) string {
	// 返回该单词出现的次数。
	return strconv.Itoa(len(values))
}
