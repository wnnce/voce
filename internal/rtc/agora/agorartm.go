package agora

/*
#cgo linux CFLAGS: -I${SRCDIR}/../../../libs/agora-server-sdk/agora_sdk/agora_rtm_sdk_c/include
#cgo linux CFLAGS: -I${SRCDIR}/../../../libs/agora-server-sdk/agora_sdk/agora_rtm_sdk_c/agora_rtm_sdk/high_level_api/include
#cgo linux CFLAGS: -Wno-incompatible-pointer-types
#cgo linux LDFLAGS: -L${SRCDIR}/../../../libs/agora-server-sdk/agora_sdk
#cgo linux LDFLAGS: -Wl,-rpath,${SRCDIR}/../../../libs/agora-server-sdk/agora_sdk
#cgo linux LDFLAGS: -lagora_rtm_sdk -lagora_rtm_sdk_c -laosl
#cgo darwin CFLAGS: -I${SRCDIR}/../../../libs/agora-server-sdk/agora_sdk/agora_rtm_sdk_c/include
#cgo darwin CFLAGS: -I${SRCDIR}/../../../libs/agora-server-sdk/agora_sdk/agora_rtm_sdk_c/agora_rtm_sdk/high_level_api/include
#cgo darwin CFLAGS: -Wno-incompatible-pointer-types
#cgo darwin LDFLAGS: -L${SRCDIR}/../../../libs/agora-server-sdk/agora_sdk_mac
#cgo darwin LDFLAGS: -Wl,-rpath,${SRCDIR}/../../../libs/agora-server-sdk/agora_sdk_mac
#cgo darwin LDFLAGS: -lAgoraRtmKit -lagora_rtm_sdk_c -laosl

#include <stdlib.h>

#include "rtm_bridge.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"
)

// rtmHandlers receives the asynchronous RTM results we act on. All callbacks run
// on SDK threads.
type rtmHandlers struct {
	OnLogin           func(code int)
	OnJoin            func(channel string, code int)
	OnJoinTopic       func(channel string, topic string, code int)
	OnPublish         func(channel string, topic string, code int)
	OnLogout          func(code int)
	OnTokenWillExpire func(channel string)
}

// rtmClient is one RTM client, identified by its account.
type rtmClient struct {
	client   unsafe.Pointer
	handler  *C.struct_C_IRtmEventHandler
	userData unsafe.Pointer
	id       uintptr
	handlers rtmHandlers
}

var (
	registryMu sync.Mutex
	registry   = make(map[uintptr]*rtmClient)
	lastID     uintptr
)

// newRTMClient creates a client for one account. Nothing is logged in yet.
func newRTMClient(appID, userID string, handlers rtmHandlers) (*rtmClient, error) {
	userData := (*C.uintptr_t)(C.malloc(C.size_t(unsafe.Sizeof(C.uintptr_t(0)))))
	if userData == nil {
		return nil, errors.New("rtm: allocate user data")
	}

	registryMu.Lock()
	lastID++
	id := lastID
	registryMu.Unlock()
	*userData = C.uintptr_t(id)

	handler := C.new_handler(unsafe.Pointer(userData))
	if handler == nil {
		C.free(unsafe.Pointer(userData))
		return nil, errors.New("rtm: create event handler")
	}

	client := &rtmClient{handler: handler, userData: unsafe.Pointer(userData), id: id, handlers: handlers}
	registryMu.Lock()
	registry[id] = client
	registryMu.Unlock()

	config := C.C_RtmConfig_New()
	if config == nil {
		client.Release()
		return nil, errors.New("rtm: allocate config")
	}
	defer C.C_RtmConfig_Delete(config)

	appIDCString := C.CString(appID)
	defer C.free(unsafe.Pointer(appIDCString))
	userIDCString := C.CString(userID)
	defer C.free(unsafe.Pointer(userIDCString))

	config.appId = appIDCString
	config.userId = userIDCString
	config.useStringUserId = C.bool(true)
	config.eventHandler = handler

	var code C.int
	client.client = C.agora_rtm_client_create(config, &code)
	if client.client == nil || code != 0 {
		client.Release()
		return nil, fmt.Errorf("rtm: create client: code %d", int(code))
	}
	return client, nil
}

// Login starts the asynchronous login; the result arrives in Handlers.OnLogin.
func (c *rtmClient) Login(token string) error {
	if c.client == nil {
		return errors.New("rtm: client is released")
	}
	tokenCString := C.CString(token)
	defer C.free(unsafe.Pointer(tokenCString))
	var requestID C.uint64_t
	if code := C.agora_rtm_client_login(c.client, tokenCString, &requestID); code != 0 {
		return fmt.Errorf("rtm: login: code %d", int(code))
	}
	return nil
}

// Logout requests logout; the result arrives in Handlers.OnLogout.
func (c *rtmClient) Logout() {
	if c.client == nil {
		return
	}
	var requestID C.uint64_t
	C.agora_rtm_client_logout(c.client, &requestID)
}

// NewStreamChannel creates one session channel. Joining is a separate call.
func (c *rtmClient) NewStreamChannel(name string) (*rtmStreamChannel, error) {
	if c.client == nil {
		return nil, errors.New("rtm: client is released")
	}
	nameCString := C.CString(name)
	defer C.free(unsafe.Pointer(nameCString))
	var code C.int
	channel := C.agora_rtm_client_create_stream_channel(c.client, nameCString, &code)
	if channel == nil || code != 0 {
		return nil, fmt.Errorf("rtm: create stream channel %q: code %d", name, int(code))
	}
	return &rtmStreamChannel{channel: channel, name: name}, nil
}

// Release frees the client and its handler.
func (c *rtmClient) Release() {
	if c.client != nil {
		C.agora_rtm_client_release(c.client)
		c.client = nil
	}
	if c.handler != nil {
		C.C_IRtmEventHandler_Delete(c.handler)
		c.handler = nil
	}
	registryMu.Lock()
	delete(registry, c.id)
	registryMu.Unlock()
	if c.userData != nil {
		C.free(c.userData)
		c.userData = nil
	}
}

// rtmStreamChannel is one stream channel owned by a binding. The topic is joined
// once and kept as a C string so publishing allocates nothing for it.
type rtmStreamChannel struct {
	channel unsafe.Pointer
	name    string
	topic   *C.char
}

// Name returns the channel name this stream channel was created with.
func (s *rtmStreamChannel) Name() string { return s.name }

// Join starts the asynchronous join; the result arrives in Handlers.OnJoin.
func (s *rtmStreamChannel) Join(token string) error {
	if s.channel == nil {
		return errors.New("rtm: stream channel is released")
	}
	options := C.C_JoinChannelOptions_New()
	if options == nil {
		return errors.New("rtm: allocate join options")
	}
	defer C.C_JoinChannelOptions_Delete(options)

	tokenCString := C.CString(token)
	defer C.free(unsafe.Pointer(tokenCString))
	options.token = tokenCString

	var requestID C.uint64_t
	C.agora_rtm_stream_channel_join(s.channel, options, &requestID)
	return nil
}

// JoinTopic starts the asynchronous topic join; the result arrives in
// Handlers.OnJoinTopic. Publishing is only valid after that.
func (s *rtmStreamChannel) JoinTopic(topic string) error {
	if s.channel == nil {
		return errors.New("rtm: stream channel is released")
	}
	options := C.C_JoinTopicOptions_New()
	if options == nil {
		return errors.New("rtm: allocate join topic options")
	}
	defer C.C_JoinTopicOptions_Delete(options)

	topicCString := C.CString(topic)

	var requestID C.uint64_t
	C.agora_rtm_stream_channel_join_topic(s.channel, topicCString, options, &requestID)

	if s.topic != nil {
		C.free(unsafe.Pointer(s.topic))
	}
	s.topic = topicCString
	return nil
}

// PublishTopicMessage publishes one message to the joined topic. The bytes are
// borrowed for the duration of the call: the SDK copies them before returning
// because the result is reported asynchronously. A rejected publish is reported
// through Handlers.OnPublish, not here.
func (s *rtmStreamChannel) PublishTopicMessage(message string) error {
	if s.channel == nil || s.topic == nil {
		return errors.New("rtm: stream channel is not joined")
	}
	if len(message) == 0 {
		return errors.New("rtm: message is empty")
	}
	options := C.C_TopicMessageOptions_New()
	if options == nil {
		return errors.New("rtm: allocate publish options")
	}
	defer C.C_TopicMessageOptions_Delete(options)
	options.messageType = C.enum_C_RTM_MESSAGE_TYPE(C.RTM_MESSAGE_TYPE_STRING)

	var requestID C.uint64_t
	C.agora_rtm_stream_channel_publish_topic_message(
		s.channel, s.topic,
		(*C.char)(unsafe.Pointer(unsafe.StringData(message))),
		C.size_t(len(message)), options, &requestID)
	return nil
}

// LeaveTopic leaves the joined topic. The result is not tracked.
func (s *rtmStreamChannel) LeaveTopic() {
	if s.channel == nil || s.topic == nil {
		return
	}
	var requestID C.uint64_t
	C.agora_rtm_stream_channel_leave_topic(s.channel, s.topic, &requestID)
}

// Release frees the stream channel.
func (s *rtmStreamChannel) Release() {
	if s.channel != nil {
		C.agora_rtm_stream_channel_release(s.channel)
		s.channel = nil
	}
	if s.topic != nil {
		C.free(unsafe.Pointer(s.topic))
		s.topic = nil
	}
}

// clientOf resolves the Go client that owns the C handler.
func clientOf(handler *C.struct_C_IRtmEventHandler) *rtmClient {
	if handler == nil || handler.userData == nil {
		return nil
	}
	id := uintptr(*(*C.uintptr_t)(handler.userData))
	registryMu.Lock()
	defer registryMu.Unlock()
	return registry[id]
}

//export cgo_onLoginResult
func cgo_onLoginResult(handler *C.struct_C_IRtmEventHandler, _requestID C.uint64_t, errorCode C.enum_C_RTM_ERROR_CODE) {
	if client := clientOf(handler); client != nil && client.handlers.OnLogin != nil {
		client.handlers.OnLogin(int(errorCode))
	}
}

//export cgo_onJoinResult
func cgo_onJoinResult(
	handler *C.struct_C_IRtmEventHandler,
	_requestID C.uint64_t,
	channelName *C.char,
	_userID *C.char,
	errorCode C.enum_C_RTM_ERROR_CODE,
) {
	client := clientOf(handler)
	if client == nil || client.handlers.OnJoin == nil {
		return
	}
	client.handlers.OnJoin(C.GoString(channelName), int(errorCode))
}

//export cgo_onJoinTopicResult
func cgo_onJoinTopicResult(
	handler *C.struct_C_IRtmEventHandler,
	_requestID C.uint64_t,
	channelName *C.char,
	_userID *C.char,
	topic *C.char,
	_meta *C.char,
	errorCode C.enum_C_RTM_ERROR_CODE,
) {
	client := clientOf(handler)
	if client == nil || client.handlers.OnJoinTopic == nil {
		return
	}
	client.handlers.OnJoinTopic(C.GoString(channelName), C.GoString(topic), int(errorCode))
}

//export cgo_onPublishResult
func cgo_onPublishResult(handler *C.struct_C_IRtmEventHandler, _requestID C.uint64_t, errorCode C.enum_C_RTM_ERROR_CODE) {
	if client := clientOf(handler); client != nil && client.handlers.OnPublish != nil {
		client.handlers.OnPublish("", "", int(errorCode))
	}
}

//export cgo_onPublishTopicMessageResult
func cgo_onPublishTopicMessageResult(
	handler *C.struct_C_IRtmEventHandler,
	_requestID C.uint64_t,
	channelName *C.char,
	topic *C.char,
	errorCode C.enum_C_RTM_ERROR_CODE,
) {
	client := clientOf(handler)
	if client == nil || client.handlers.OnPublish == nil {
		return
	}
	client.handlers.OnPublish(C.GoString(channelName), C.GoString(topic), int(errorCode))
}

//export cgo_onLogoutResult
func cgo_onLogoutResult(handler *C.struct_C_IRtmEventHandler, _requestID C.uint64_t, errorCode C.enum_C_RTM_ERROR_CODE) {
	if client := clientOf(handler); client != nil && client.handlers.OnLogout != nil {
		client.handlers.OnLogout(int(errorCode))
	}
}

//export cgo_onTokenPrivilegeWillExpire
func cgo_onTokenPrivilegeWillExpire(handler *C.struct_C_IRtmEventHandler, channelName *C.char) {
	if client := clientOf(handler); client != nil && client.handlers.OnTokenWillExpire != nil {
		client.handlers.OnTokenWillExpire(C.GoString(channelName))
	}
}
