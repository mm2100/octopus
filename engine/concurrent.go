package engine

import (
	"fmt"
	"Michael-Min/octopus/config"
	"Michael-Min/octopus/bloom"
	"Michael-Min/octopus/gredis"
	pb "Michael-Min/octopus/proto"
	"log"
	"time"
)

type ConcurrentEngine struct {
	Scheduler        Scheduler
	WorkerCount      int
	ItemChan         chan pb.Item
	RequestProcessor Processor
}

type Processor func(Request) (ParseResult, error)

type Scheduler interface {
	ReadyNotifier
	Submit(Request)
	WorkerChan() chan Request
	Run()
}

type ReadyNotifier interface {
	WorkerReady(chan Request)
}

var (
	rateLimiter = time.Tick(
		20*time.Second / config.Qps)
)

func (e *ConcurrentEngine) Run(seeds ...Request) {
	out := make(chan ParseResult)
	gredis.Setup()
	e.Scheduler.Run()
	for i := 0; i < e.WorkerCount; i++ {
		e.createWorker(e.Scheduler.WorkerChan(),
			out, e.Scheduler)
	}

	for _, r := range seeds {
		// 去重
		if isDuplicate(r.Url) {
			continue
		}
		e.Scheduler.Submit(r)
	}

	for {
		fmt.Println("Loop: fetch result <==>")
		result := <-out
		for _, item := range result.Items {
			go func(i pb.Item) {
				e.ItemChan <- i
				fmt.Printf("Iterm:%#v\n",i)
			}(*item)
		}

		for _, request := range result.Requests {
			if isDuplicate(request.Url) {
				continue
			}
			e.Scheduler.Submit(request)
		}
	}
}

func (e *ConcurrentEngine) createWorker(
	in chan Request,
	out chan ParseResult, ready ReadyNotifier) {
	go func() {
		for {
			ready.WorkerReady(in)
			fmt.Println("Process wait request...")
			request := <-in
			log.Printf("Process load request...")
			<-rateLimiter
			log.Printf("Process fetch request start...")
			result, err := e.RequestProcessor(
				request)
			if err != nil {
				log.Printf("Error: Process return==> %v\n",err)
				continue
			}
			log.Printf("Process finish...")
			// 这里是获取的结果
			out <- result
		}
	}()
}

var visitedUrls = make(map[string]bool)

//func isDuplicate(url string) bool {
//	if visitedUrls[url] {
//		return true
//	}
//
//	visitedUrls[url] = true
//	return false
//}

func isDuplicate(url string) bool {
	b, err := bloom.NewBloomFilter().IsContains(url)
	if err != nil{
		log.Println("IsContains failed:", err.Error())
		return false
	}
	if b == 1{
		fmt.Printf("Existed url: %s\n",url)
		return true
	}
	err = bloom.NewBloomFilter().Insert(url)
	if err != nil{
		log.Println("Insert failed:%s", err.Error())
		return false
	}
	return false
}
