package agora

import (
	"fmt"
	"time"

	rtctokenbuilder "github.com/AgoraIO/Tools/DynamicKey/AgoraDynamicKey/go/src/rtctokenbuilder2"
	rtmtokenbuilder "github.com/AgoraIO/Tools/DynamicKey/AgoraDynamicKey/go/src/rtmtokenbuilder2"
)

// Credentials is everything a client needs to attach to one session's channel.
// UID is signed by the server, so the client must join RTC and log in to RTM
// with exactly this account.
type Credentials struct {
	AppID    string
	Channel  string
	UID      string
	RTCToken string
	RTMToken string
	ExpireAt time.Time
}

// tokenBuilder signs RTC and RTM tokens with the app certificate. Agora treats
// the expiry arguments as "seconds from now", not as absolute timestamps.
type tokenBuilder struct {
	appID           string
	appCertificate  string
	tokenExpire     time.Duration
	privilegeExpire time.Duration
	rtmTokenExpire  time.Duration
}

func (b tokenBuilder) build(channel, uid string) (Credentials, error) {
	credentials := Credentials{
		AppID:    b.appID,
		Channel:  channel,
		UID:      uid,
		ExpireAt: time.Now().Add(b.expireWindow()),
	}
	if b.appCertificate == "" {
		// An app without a certificate accepts an empty token; there is nothing
		// to sign, so the client joins with what it has.
		return credentials, nil
	}

	rtcToken, err := rtctokenbuilder.BuildTokenWithUserAccount(
		b.appID, b.appCertificate, channel, uid,
		rtctokenbuilder.RolePublisher,
		seconds(b.tokenExpire), seconds(b.privilegeExpire),
	)
	if err != nil {
		return Credentials{}, fmt.Errorf("rtc: build rtc token: %w", err)
	}
	rtmToken, err := rtmtokenbuilder.BuildToken(b.appID, b.appCertificate, uid, seconds(b.rtmTokenExpire))
	if err != nil {
		return Credentials{}, fmt.Errorf("rtc: build rtm token: %w", err)
	}

	credentials.RTCToken = rtcToken
	credentials.RTMToken = rtmToken
	return credentials, nil
}

// rtmToken signs the RTM token for one account. The token is user scoped, not
// channel scoped, so the process logs in once with it.
func (b tokenBuilder) rtmToken(uid string) (string, error) {
	if b.appCertificate == "" {
		return "", nil
	}
	token, err := rtmtokenbuilder.BuildToken(b.appID, b.appCertificate, uid, seconds(b.rtmTokenExpire))
	if err != nil {
		return "", fmt.Errorf("rtc: build rtm token: %w", err)
	}
	return token, nil
}

// expireWindow is the shortest lifetime among the three tokens: credentials are
// only usable while every token in them is still valid.
func (b tokenBuilder) expireWindow() time.Duration {
	window := b.tokenExpire
	if b.privilegeExpire < window {
		window = b.privilegeExpire
	}
	if b.rtmTokenExpire < window {
		window = b.rtmTokenExpire
	}
	return window
}

func seconds(d time.Duration) uint32 {
	return uint32(d / time.Second)
}
