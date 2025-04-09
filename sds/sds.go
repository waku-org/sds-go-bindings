package sds

/*
	#cgo LDFLAGS: -L../third_party/nim-sds/build/ -lsds
	#cgo LDFLAGS: -L../third_party/nim-sds -Wl,-rpath,../third_party/nim-sds/build/

	#include "../third_party/nim-sds/library/libsds.h"
	#include <stdio.h>
	#include <stdlib.h>

	extern void globalEventCallback(int ret, char* msg, size_t len, void* userData);

	typedef struct {
		int ret;
		char* msg;
		size_t len;
		void* ffiWg;
	} Resp;

	static void* allocResp(void* wg) {
		Resp* r = calloc(1, sizeof(Resp));
		r->ffiWg = wg;
		return r;
	}

	static void freeResp(void* resp) {
		if (resp != NULL) {
			free(resp);
		}
	}

	static char* getMyCharPtr(void* resp) {
		if (resp == NULL) {
			return NULL;
		}
		Resp* m = (Resp*) resp;
		return m->msg;
	}

	static size_t getMyCharLen(void* resp) {
		if (resp == NULL) {
			return 0;
		}
		Resp* m = (Resp*) resp;
		return m->len;
	}

	static int getRet(void* resp) {
		if (resp == NULL) {
			return 0;
		}
		Resp* m = (Resp*) resp;
		return m->ret;
	}

	// resp must be set != NULL in case interest on retrieving data from the callback
	void GoCallback(int ret, char* msg, size_t len, void* resp);

	static void* cGoNewReliabilityManager(const char* channelId, void* resp) {
		// We pass NULL because we are not interested in retrieving data from this callback
		void* ret = NewReliabilityManager(channelId, (SdsCallBack) GoCallback, resp);
		return ret;
	}
*/
import "C"
import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
	"unsafe"
)

const requestTimeout = 30 * time.Second

//export GoCallback
func GoCallback(ret C.int, msg *C.char, len C.size_t, resp unsafe.Pointer) {
	if resp != nil {
		m := (*C.Resp)(resp)
		m.ret = ret
		m.msg = msg
		m.len = len
		wg := (*sync.WaitGroup)(m.ffiWg)
		wg.Done()
	}
}

// ReliabilityManager represents an instance of a nim-sds ReliabilityManager
type ReliabilityManager struct {
	rmCtx     unsafe.Pointer
	rmName    string
	channelId string
}

func NewReliabilityManager(channelId string, rmName string) (*ReliabilityManager, error) {
	Debug("Creating new WakuNode: %v", rmName)
	rm := &ReliabilityManager{
		channelId: channelId,
		rmName:    rmName,
	}

	wg := sync.WaitGroup{}

	var cChannelId = C.CString(string(channelId))
	var resp = C.allocResp(unsafe.Pointer(&wg))

	defer C.free(unsafe.Pointer(cChannelId))
	defer C.freeResp(resp)

	if C.getRet(resp) != C.RET_OK {
		errMsg := C.GoStringN(C.getMyCharPtr(resp), C.int(C.getMyCharLen(resp)))
		Error("error NewReliabilityManager for %s: %v", rmName, errMsg)
		return nil, errors.New(errMsg)
	}

	wg.Add(1)
	rm.rmCtx = C.cGoNewReliabilityManager(cChannelId, resp)
	wg.Wait()

	// C.cGoWakuSetEventCallback(n.wakuCtx) TODO
	registerReliabilityManager(rm)

	Debug("Successfully created WakuNode: %s", rmName)
	return rm, nil
}

// The event callback sends back the rm ctx to know to which
// rm is the event being emited for. Since we only have a global
// callback in the go side, We register all the rm's that we create
// so we can later obtain which instance of `ReliabilityManager` it should
// be invoked depending on the ctx received

var rmRegistry map[unsafe.Pointer]*ReliabilityManager

func init() {
	rmRegistry = make(map[unsafe.Pointer]*ReliabilityManager)
}

func registerReliabilityManager(rm *ReliabilityManager) {
	_, ok := rmRegistry[rm.rmCtx]
	if !ok {
		rmRegistry[rm.rmCtx] = rm
	}
}

func unregisterReliabilityManager(rm *ReliabilityManager) {
	delete(rmRegistry, rm.rmCtx)
}

//export globalEventCallback
func globalEventCallback(callerRet C.int, msg *C.char, len C.size_t, userData unsafe.Pointer) {
	if callerRet == C.RET_OK {
		eventStr := C.GoStringN(msg, C.int(len))
		rm, ok := rmRegistry[userData] // userData contains node's ctx
		if ok {
			rm.OnEvent(eventStr)
		}
	} else {
		if len != 0 {
			errMsg := C.GoStringN(msg, C.int(len))
			Error("globalEventCallback retCode not ok, retCode: %v: %v", callerRet, errMsg)
		} else {
			Error("globalEventCallback retCode not ok, retCode: %v", callerRet)
		}
	}
}

type jsonEvent struct {
	EventType string `json:"eventType"`
}

func (n *ReliabilityManager) OnEvent(eventStr string) {
	jsonEvent := jsonEvent{}
	err := json.Unmarshal([]byte(eventStr), &jsonEvent)
	if err != nil {
		Error("could not unmarshal sds event string: %v", err)

		return
	}

	switch jsonEvent.EventType {
	case "event 1":
		fmt.Println("-------- received event 1")
	case "event 2":
		fmt.Println("-------- received event 1")
	}
}
