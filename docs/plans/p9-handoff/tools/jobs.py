import json,sys
from collections import Counter
jobs=json.load(open(sys.argv[1]))["jobs"]["jobs"]
print(Counter((j["status"],j.get("conclusion")) for j in jobs))
for j in jobs:
  if j.get("conclusion") not in ("success","skipped"):
    fs=[s["name"] for s in j.get("steps",[]) if s.get("conclusion")=="failure"]
    print(j["id"], j["status"], j.get("conclusion"), j["name"], fs)
