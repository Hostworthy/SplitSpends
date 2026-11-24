# Seeds demo data for README screenshots through the public API.
import json, re, time, urllib.request
import os
B = os.environ.get("BASE_URL", "http://localhost:8080") + "/api/v1"
LOG = os.environ["SERVER_LOG"]
def call(method, path, body=None, token=None):
    req = urllib.request.Request(B + path, data=json.dumps(body).encode() if body is not None else None, method=method)
    req.add_header("Content-Type", "application/json")
    if token: req.add_header("Authorization", "Bearer " + token)
    with urllib.request.urlopen(req) as r:
        t = r.read().decode(); return json.loads(t) if t else None
def login(email, name):
    call("POST", "/auth/otp/request", {"email": email}); time.sleep(0.3)
    code = re.findall(r'to=%s subject="(\d{6})' % re.escape(email), open(LOG).read())[-1]
    return call("POST", "/auth/otp/verify", {"email": email, "code": code, "name": name})["data"]
ada = login("ada@example.com", "Ada Lovelace"); T = ada["token"]; me = ada["user"]["id"]
call("PATCH", "/me", {"defaultCurrency": "EUR", "avatarColor": "#00897B"}, T)
g = call("POST", "/groups", {"name": "Lisbon trip", "kind": "trip", "emoji": "✈️", "color": "#00897B", "defaultCurrency": "EUR",
    "members": [{"email": "ben@example.com", "name": "Ben Carter"}, {"email": "cleo@example.com", "name": "Cleo Nakamura"}, {"name": "Dev Patel"}]}, T)["data"]
ids = {m["name"].split()[0]: m["id"] for m in g["members"]}; ids["Ada"] = me
everyone = [{"userId": i} for i in ids.values()]
def exp(desc, total, payer, cat, date, splits=None, method="equal", group=g["id"], items=None, tax=0, tip=0, currency="EUR"):
    body = {"groupId": group, "description": desc, "currency": currency, "total": total, "category": cat, "date": date,
            "splitMethod": method, "payers": [{"userId": ids[payer], "amount": total}]}
    if items: body.update(items=items, tax=tax, tip=tip)
    else: body["splits"] = splits or everyone
    return call("POST", "/expenses", body, T)["data"]
exp("Flights to Lisbon", 64000, "Ada", "travel.flights", "2026-10-02")
exp("Apartment in the old town", 92000, "Ben", "travel.lodging", "2026-10-03")
exp("Tram 28 day passes", 2560, "Cleo", "transport.transit", "2026-10-04")
exp("Custard tarts", 1240, "Dev", "food.coffee", "2026-10-04")
exp("Dinner at the market", 13240, "Ada", "food.dining", "2026-10-05", method="itemized", items=[
    {"name": "Grilled octopus", "amount": 2800, "userIds": [ids["Ada"], ids["Ben"]]},
    {"name": "Salt cod", "amount": 2400, "userIds": [ids["Cleo"]]},
    {"name": "Seafood rice", "amount": 4200, "userIds": [ids["Dev"], ids["Ada"]]},
    {"name": "Wine", "amount": 2600, "userIds": list(ids.values())}], tax=640, tip=600)
exp("Day trip to the coast", 18000, "Ben", "travel.activities", "2026-10-06", method="shares",
    splits=[{"userId": ids["Ada"], "value": 1}, {"userId": ids["Ben"], "value": 2}, {"userId": ids["Cleo"], "value": 1}, {"userId": ids["Dev"], "value": 2}])
exp("Groceries", 5420, "Cleo", "food.groceries", "2026-10-07")
exp("Live music night", 9600, "Dev", "entertainment.music", "2026-10-08", method="percent",
    splits=[{"userId": ids["Ada"], "value": 25}, {"userId": ids["Ben"], "value": 25}, {"userId": ids["Cleo"], "value": 25}, {"userId": ids["Dev"], "value": 25}])
call("POST", "/payments", {"groupId": g["id"], "fromUserId": ids["Dev"], "toUserId": me, "amount": 10000, "currency": "EUR", "date": "2026-10-12"}, T)
flat = call("POST", "/groups", {"name": "Flat 4B", "kind": "home", "emoji": "🏠", "color": "#3949AB", "defaultCurrency": "EUR",
    "members": [{"email": "ben@example.com"}]}, T)["data"]
two = [{"userId": me}, {"userId": ids["Ben"]}]
for m, d in (("09", "2026-09-01"), ("10", "2026-10-01"), ("11", "2026-11-01")):
    exp("Rent", 180000, "Ada", "home.rent", d, splits=two, group=flat["id"])
exp("Electricity", 9650, "Ben", "home.utilities", "2026-11-12", splits=two, group=flat["id"])
exp("Internet", 4500, "Ada", "home.internet", "2026-11-15", splits=two, group=flat["id"])
exp("Cleaning supplies", 2380, "Ben", "home.supplies", "2026-11-18", splits=two, group=flat["id"])
exp("Coffee", 980, "Ada", "food.coffee", "2026-11-20", splits=[{"userId": me}, {"userId": ids["Cleo"]}], group=None)
print(json.dumps({"lisbon": g["id"], "flat": flat["id"]}))
