#include "rtm_bridge.h"
#include "_cgo_export.h"

// The SDK may call any slot; unset ones must still be callable.
static void noop_onMessageEvent(struct C_IRtmEventHandler *handler, const struct C_MessageEvent *event) {}
static void noop_onPresenceEvent(struct C_IRtmEventHandler *handler, const struct C_PresenceEvent *event) {}
static void noop_onTopicEvent(struct C_IRtmEventHandler *handler, const struct C_TopicEvent *event) {}
static void noop_onLockEvent(struct C_IRtmEventHandler *handler, const struct C_LockEvent *event) {}
static void noop_onStorageEvent(struct C_IRtmEventHandler *handler, const struct C_StorageEvent *event) {}
static void noop_onTokenEvent(struct C_IRtmEventHandler *handler, const struct C_TokenEvent *event) {}
static void noop_onLeaveResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, const char *channelName, const char *userId, enum C_RTM_ERROR_CODE errorCode) {}
static void noop_onLeaveTopicResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, const char *channelName, const char *userId, const char *topic, const char *meta, enum C_RTM_ERROR_CODE errorCode) {}
static void noop_onSubscribeTopicResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, const char *channelName, const char *userId, const char *topic, struct C_UserList succeedUsers, struct C_UserList failedUsers, enum C_RTM_ERROR_CODE errorCode) {}
static void noop_onSubscribeResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, const char *channelName, enum C_RTM_ERROR_CODE errorCode) {}
static void noop_onUnsubscribeResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, const char *channelName, enum C_RTM_ERROR_CODE errorCode) {}
static void noop_onSetChannelMetadataResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, const char *channelName, enum C_RTM_CHANNEL_TYPE channelType, enum C_RTM_ERROR_CODE errorCode) {}
static void noop_onUpdateChannelMetadataResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, const char *channelName, enum C_RTM_CHANNEL_TYPE channelType, enum C_RTM_ERROR_CODE errorCode) {}
static void noop_onRemoveChannelMetadataResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, const char *channelName, enum C_RTM_CHANNEL_TYPE channelType, enum C_RTM_ERROR_CODE errorCode) {}
static void noop_onGetChannelMetadataResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, const char *channelName, enum C_RTM_CHANNEL_TYPE channelType, const struct C_Metadata *data, enum C_RTM_ERROR_CODE errorCode) {}
static void noop_onSetUserMetadataResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, const char *userId, enum C_RTM_ERROR_CODE errorCode) {}
static void noop_onUpdateUserMetadataResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, const char *userId, enum C_RTM_ERROR_CODE errorCode) {}
static void noop_onRemoveUserMetadataResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, const char *userId, enum C_RTM_ERROR_CODE errorCode) {}
static void noop_onGetUserMetadataResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, const char *userId, const struct C_Metadata *data, enum C_RTM_ERROR_CODE errorCode) {}
static void noop_onSubscribeUserMetadataResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, const char *userId, enum C_RTM_ERROR_CODE errorCode) {}
static void noop_onSetLockResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, const char *channelName, enum C_RTM_CHANNEL_TYPE channelType, const char *lockName, enum C_RTM_ERROR_CODE errorCode) {}
static void noop_onRemoveLockResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, const char *channelName, enum C_RTM_CHANNEL_TYPE channelType, const char *lockName, enum C_RTM_ERROR_CODE errorCode) {}
static void noop_onReleaseLockResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, const char *channelName, enum C_RTM_CHANNEL_TYPE channelType, const char *lockName, enum C_RTM_ERROR_CODE errorCode) {}
static void noop_onAcquireLockResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, const char *channelName, enum C_RTM_CHANNEL_TYPE channelType, const char *lockName, enum C_RTM_ERROR_CODE errorCode, const char *errorDetails) {}
static void noop_onRevokeLockResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, const char *channelName, enum C_RTM_CHANNEL_TYPE channelType, const char *lockName, enum C_RTM_ERROR_CODE errorCode) {}
static void noop_onGetLocksResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, const char *channelName, enum C_RTM_CHANNEL_TYPE channelType, const struct C_LockDetail *lockDetailList, const size_t count, enum C_RTM_ERROR_CODE errorCode) {}
static void noop_onWhoNowResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, const struct C_UserState *userStateList, const size_t count, const char *nextPage, enum C_RTM_ERROR_CODE errorCode) {}
static void noop_onGetOnlineUsersResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, const struct C_UserState *userStateList, const size_t count, const char *nextPage, enum C_RTM_ERROR_CODE errorCode) {}
static void noop_onWhereNowResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, const struct C_ChannelInfo *channels, const size_t count, enum C_RTM_ERROR_CODE errorCode) {}
static void noop_onGetUserChannelsResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, const struct C_ChannelInfo *channels, const size_t count, enum C_RTM_ERROR_CODE errorCode) {}
static void noop_onPresenceSetStateResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, enum C_RTM_ERROR_CODE errorCode) {}
static void noop_onPresenceRemoveStateResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, enum C_RTM_ERROR_CODE errorCode) {}
static void noop_onPresenceGetStateResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, const struct C_UserState *state, enum C_RTM_ERROR_CODE errorCode) {}
static void noop_onLinkStateEvent(struct C_IRtmEventHandler *handler, const struct C_LinkStateEvent *event) {}
static void noop_onGetHistoryMessagesResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, const struct C_HistoryMessage *messageList, const size_t count, const uint64_t newStart, enum C_RTM_ERROR_CODE errorCode) {}
static void noop_onRenewTokenResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, enum C_RTM_SERVICE_TYPE serverType, const char *channelName, enum C_RTM_ERROR_CODE errorCode) {}
static void noop_onUnsubscribeTopicResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, const char *channelName, const char *topic, enum C_RTM_ERROR_CODE errorCode) {}
static void noop_onGetSubscribedUserListResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, const char *channelName, const char *topic, struct C_UserList users, enum C_RTM_ERROR_CODE errorCode) {}
static void noop_onUnsubscribeUserMetadataResult(struct C_IRtmEventHandler *handler, const uint64_t requestId, const char *userId, enum C_RTM_ERROR_CODE errorCode) {}

struct C_IRtmEventHandler *new_handler(void *userData) {
  struct C_IRtmEventHandler *h = C_IRtmEventHandler_New(userData);
  if (h == NULL) { return NULL; }
  // The constructor does not keep the argument, so set it explicitly.
  h->userData = userData;
  h->onMessageEvent = noop_onMessageEvent;
  h->onPresenceEvent = noop_onPresenceEvent;
  h->onTopicEvent = noop_onTopicEvent;
  h->onLockEvent = noop_onLockEvent;
  h->onStorageEvent = noop_onStorageEvent;
  h->onTokenEvent = noop_onTokenEvent;
  h->onJoinResult = cgo_onJoinResult;
  h->onLeaveResult = noop_onLeaveResult;
  h->onJoinTopicResult = cgo_onJoinTopicResult;
  h->onLeaveTopicResult = noop_onLeaveTopicResult;
  h->onSubscribeTopicResult = noop_onSubscribeTopicResult;
  h->onTokenPrivilegeWillExpire = cgo_onTokenPrivilegeWillExpire;
  h->onSubscribeResult = noop_onSubscribeResult;
  h->onUnsubscribeResult = noop_onUnsubscribeResult;
  h->onPublishResult = cgo_onPublishResult;
  h->onLoginResult = cgo_onLoginResult;
  h->onSetChannelMetadataResult = noop_onSetChannelMetadataResult;
  h->onUpdateChannelMetadataResult = noop_onUpdateChannelMetadataResult;
  h->onRemoveChannelMetadataResult = noop_onRemoveChannelMetadataResult;
  h->onGetChannelMetadataResult = noop_onGetChannelMetadataResult;
  h->onSetUserMetadataResult = noop_onSetUserMetadataResult;
  h->onUpdateUserMetadataResult = noop_onUpdateUserMetadataResult;
  h->onRemoveUserMetadataResult = noop_onRemoveUserMetadataResult;
  h->onGetUserMetadataResult = noop_onGetUserMetadataResult;
  h->onSubscribeUserMetadataResult = noop_onSubscribeUserMetadataResult;
  h->onSetLockResult = noop_onSetLockResult;
  h->onRemoveLockResult = noop_onRemoveLockResult;
  h->onReleaseLockResult = noop_onReleaseLockResult;
  h->onAcquireLockResult = noop_onAcquireLockResult;
  h->onRevokeLockResult = noop_onRevokeLockResult;
  h->onGetLocksResult = noop_onGetLocksResult;
  h->onWhoNowResult = noop_onWhoNowResult;
  h->onGetOnlineUsersResult = noop_onGetOnlineUsersResult;
  h->onWhereNowResult = noop_onWhereNowResult;
  h->onGetUserChannelsResult = noop_onGetUserChannelsResult;
  h->onPresenceSetStateResult = noop_onPresenceSetStateResult;
  h->onPresenceRemoveStateResult = noop_onPresenceRemoveStateResult;
  h->onPresenceGetStateResult = noop_onPresenceGetStateResult;
  h->onLinkStateEvent = noop_onLinkStateEvent;
  h->onGetHistoryMessagesResult = noop_onGetHistoryMessagesResult;
  h->onLogoutResult = cgo_onLogoutResult;
  h->onRenewTokenResult = noop_onRenewTokenResult;
  h->onPublishTopicMessageResult = cgo_onPublishTopicMessageResult;
  h->onUnsubscribeTopicResult = noop_onUnsubscribeTopicResult;
  h->onGetSubscribedUserListResult = noop_onGetSubscribedUserListResult;
  h->onUnsubscribeUserMetadataResult = noop_onUnsubscribeUserMetadataResult;
  return h;
}
