# %%
import os
import json
import matplotlib.pyplot as plt
import numpy as np

NAME = "unforwarded-search-timer"
FOLDER = "results/" + NAME

labels = [2, 4, 6, 8, 10]
p01ValuesTtfb = {2: [], 4: [], 6: [], 8: [], 10: []}
p02ValuesTtfb = {2: [], 4: [], 6: [], 8: [], 10: []}
p01ValuesTrigger = {2: [], 4: [], 6: [], 8: [], 10: []}
p02ValuesTrigger = {2: [], 4: [], 6: [], 8: [], 10: []}

for subdir, _, files in os.walk(FOLDER):
    if "results.out" not in files:
        continue
    peerNumber = subdir.split("/")[-1]
    dataDirName = subdir.split("/")[-3]
    pStr = dataDirName.split("_")[0]
    uStr = dataDirName.split("_")[1]
    p = float(pStr[2:-1])
    u = int(uStr[2:-1])
    with open(subdir + "/results.out") as f:
        num_run_trigger = 0
        for l in f:
            datapoint = json.loads(l)
            # print(datapoint)
            if datapoint["name"] == "time-to-fetch-ms":
                if p == 0.1:
                    p01ValuesTtfb[u].append(float(datapoint["measures"]["value"]) / 1000.0)
                if p == 0.2:
                    p02ValuesTtfb[u].append(float(datapoint["measures"]["value"]) / 1000.0)
            if datapoint["name"] == "unforwarded-search-counter":
                value = int(datapoint["measures"]["value"])
                if p == 0.1:
                    if len(p01ValuesTrigger[u]) < num_run_trigger + 1:
                        p01ValuesTrigger[u].append([])
                    if value > 0:
                        p01ValuesTrigger[u][num_run_trigger].append(1)
                    else:
                        p01ValuesTrigger[u][num_run_trigger].append(0)
                    num_run_trigger += 1
                if p == 0.2:
                    if len(p02ValuesTrigger[u]) < num_run_trigger + 1:
                        p02ValuesTrigger[u].append([])
                    if value > 0:
                        p02ValuesTrigger[u][num_run_trigger].append(1)
                    else:
                        p02ValuesTrigger[u][num_run_trigger].append(0)
                    num_run_trigger += 1

p01ValuesTriggerPerc = {2: [], 4: [], 6: [], 8: [], 10: []}
p02ValuesTriggerPerc = {2: [], 4: [], 6: [], 8: [], 10: []}
for k in p01ValuesTrigger.keys():
    for v in p01ValuesTrigger[k]:
        sum = 0
        count = 0
        for t in v:
            count += 1
            sum += t
        p01ValuesTriggerPerc[k].append(sum / count)
for k in p02ValuesTrigger.keys():
    for v in p02ValuesTrigger[k]:
        sum = 0
        count = 0
        for t in v:
            count += 1
            sum += t
        p02ValuesTriggerPerc[k].append(sum / count)

# %%
fig, ax = plt.subplots(figsize=[3.6, 4.8])
pos = np.array(list(p01ValuesTtfb.keys()))

bpp01 = ax.boxplot(p01ValuesTtfb.values(), widths=0.5, patch_artist=True,
    showmeans=False, showfliers=True, sym="+",
    positions=pos-0.3,
    medianprops={"color": "white", "linewidth": 1.0},
    boxprops={"facecolor": "#ae4132", "edgecolor": "white",
                "linewidth": 0.5},
    whiskerprops={"color": "#ae4132", "linewidth": 1.5},
    capprops={"color": "#ae4132", "linewidth": 1.5})
bpp02 = ax.boxplot(p02ValuesTtfb.values(), widths=0.5, patch_artist=True,
    showmeans=False, showfliers=True, sym="+",
    positions=pos+0.3,
    medianprops={"color": "white", "linewidth": 1.0},
    boxprops={"facecolor": "#56517e", "edgecolor": "white",
                "linewidth": 0.5},
    whiskerprops={"color": "#56517e", "linewidth": 1.5},
    capprops={"color": "#56517e", "linewidth": 1.5})

plt.xticks(labels, labels)
ax.set_ylabel("TTFB (s)")
ax.set_xlabel("unforwarded search timer duration (s)")

# grid configuration
plt.yticks(np.arange(1,16,0.5), minor=True) 
plt.tick_params(which='minor', length=0)  
plt.grid(axis="y")
plt.grid(axis="y", which="minor")

ax.legend([bpp01["boxes"][0], bpp02["boxes"][0]], ["p=0.1", "p=0.2"], loc="upper left")

# plt.savefig("plots/" + NAME + "-ttfb.svg", format="svg")
plt.show()

# %%
fig, ax = plt.subplots(figsize=[3.6, 4.8])
pos = np.array(list(p01ValuesTriggerPerc.keys()))

bpp01 = ax.boxplot(p01ValuesTriggerPerc.values(), widths=0.5, patch_artist=True,
    showmeans=False, showfliers=True, sym="+",
    positions=pos-0.3,
    medianprops={"color": "white", "linewidth": 1.0},
    boxprops={"facecolor": "#ae4132", "edgecolor": "white",
                "linewidth": 0.5},
    whiskerprops={"color": "#ae4132", "linewidth": 1.5},
    capprops={"color": "#ae4132", "linewidth": 1.5})
bpp02 = ax.boxplot(p02ValuesTriggerPerc.values(), widths=0.5, patch_artist=True,
    showmeans=False, showfliers=True, sym="+",
    positions=pos+0.3,
    medianprops={"color": "white", "linewidth": 1.0},
    boxprops={"facecolor": "#56517e", "edgecolor": "white",
                "linewidth": 0.5},
    whiskerprops={"color": "#56517e", "linewidth": 1.5},
    capprops={"color": "#56517e", "linewidth": 1.5})

plt.xticks(labels, labels)
ax.set_ylabel("fraction of triggered unforwarded search timers")
ax.set_xlabel("unforwarded search timer duration (s)")

# grid configuration
plt.yticks(np.arange(0,0.6,0.05), minor=True) 
plt.tick_params(which='minor', length=0)  
plt.grid(axis="y")
plt.grid(axis="y", which="minor")

ax.legend([bpp01["boxes"][0], bpp02["boxes"][0]], ["p=0.1", "p=0.2"], loc="upper right")

# plt.savefig("plots/" + NAME + "-trigger.svg", format="svg")
plt.show()
# %%

print("MEDIAN TTFB")
for k, v in p01ValuesTtfb.items():
    print(f"RaWa [p=0.1,u={k}]: {np.median(v)}")
for k, v in p02ValuesTtfb.items():
    print(f"RaWa [p=0.2,u={k}]: {np.median(v)}")

# %%
