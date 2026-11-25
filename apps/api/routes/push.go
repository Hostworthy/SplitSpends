package routes

import (
	"net/http"
	"strings"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/push"
	"github.com/Hostworthy/SplitSpends/packages/platform"
)

func (a *API) listPushSubscriptions(w http.ResponseWriter, r *http.Request) error {
	subs, err := a.svc.Push.List(r.Context(), me(r))
	if err != nil {
		return err
	}
	out := make([]platform.PushSubscription, 0, len(subs))
	for _, s := range subs {
		out = append(out, toPushSubscription(s))
	}
	return paged(w, r, out)
}

func (a *API) createPushSubscription(w http.ResponseWriter, r *http.Request) error {
	in, err := decode[platform.PushSubscriptionCreate](w, r)
	if err != nil {
		return err
	}
	label := in.Label
	if strings.TrimSpace(label) == "" {
		label = deviceLabel(r.UserAgent())
	}
	s, created, err := a.svc.Push.Register(r.Context(), me(r), session(r), push.Input{Endpoint: in.Endpoint, P256dh: in.Keys.P256dh,
		Auth: in.Keys.Auth, Platform: in.Platform, Label: label})
	if err != nil {
		return err
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	return details(w, status, toPushSubscription(s))
}

func (a *API) deletePushSubscription(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "subscriptionId", "Device")
	if err != nil {
		return err
	}
	if err := a.svc.Push.Remove(r.Context(), me(r), id); err != nil {
		return err
	}
	return noContent(w)
}

func (a *API) testPush(w http.ResponseWriter, r *http.Request) error {
	report, err := a.svc.Push.Test(r.Context(), me(r))
	if err != nil {
		return err
	}
	return details(w, http.StatusOK, platform.PushTest{Sent: report.Sent, Failed: report.Failed, Removed: report.Removed})
}

// deviceLabel names a device from its User-Agent when the app sent no
// label, for example "Firefox on Linux".
func deviceLabel(ua string) string {
	browser := ""
	for _, b := range []struct{ token, name string }{
		{"Edg/", "Edge"}, {"OPR/", "Opera"}, {"Firefox/", "Firefox"}, {"Chrome/", "Chrome"}, {"Safari/", "Safari"}, {"Dart/", "App"},
	} {
		if strings.Contains(ua, b.token) {
			browser = b.name
			break
		}
	}
	system := ""
	for _, s := range []struct{ token, name string }{
		{"Android", "Android"}, {"iPhone", "iPhone"}, {"iPad", "iPad"}, {"Windows", "Windows"}, {"Mac OS", "macOS"}, {"CrOS", "ChromeOS"}, {"Linux", "Linux"},
	} {
		if strings.Contains(ua, s.token) {
			system = s.name
			break
		}
	}
	switch {
	case browser != "" && system != "":
		return browser + " on " + system
	case browser != "":
		return browser
	case system != "":
		return system
	}
	return "Device"
}
