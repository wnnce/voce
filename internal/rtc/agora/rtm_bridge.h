#ifndef VOCE_RTM_BRIDGE_H
#define VOCE_RTM_BRIDGE_H

#include "C_IAgoraRtmClient.h"
#include "C_IAgoraStreamChannel.h"

// Implemented in Go; cgo generates the definitions.
// The signatures must match what cgo exports.
void cgo_onLoginResult(struct C_IRtmEventHandler *handler, uint64_t requestId, enum C_RTM_ERROR_CODE errorCode);
void cgo_onJoinResult(struct C_IRtmEventHandler *handler, uint64_t requestId, char *channelName, char *userId, enum C_RTM_ERROR_CODE errorCode);
void cgo_onJoinTopicResult(struct C_IRtmEventHandler *handler, uint64_t requestId, char *channelName, char *userId, char *topic, char *meta, enum C_RTM_ERROR_CODE errorCode);
void cgo_onPublishResult(struct C_IRtmEventHandler *handler, uint64_t requestId, enum C_RTM_ERROR_CODE errorCode);
void cgo_onPublishTopicMessageResult(struct C_IRtmEventHandler *handler, uint64_t requestId, char *channelName, char *topic, enum C_RTM_ERROR_CODE errorCode);
void cgo_onLogoutResult(struct C_IRtmEventHandler *handler, uint64_t requestId, enum C_RTM_ERROR_CODE errorCode);
void cgo_onTokenPrivilegeWillExpire(struct C_IRtmEventHandler *handler, char *channelName);

// Builds a handler whose unset slots are no-ops.
struct C_IRtmEventHandler *new_handler(void *userData);

#endif
