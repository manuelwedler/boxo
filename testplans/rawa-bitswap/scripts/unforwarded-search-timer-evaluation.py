# %%
import os
import json
import matplotlib.pyplot as plt
import numpy as np

NAME = "unforwarded-search-timer"
FOLDER = "results/" + NAME

labels = [2, 4, 6, 8, 10]
p01ValuesTtfb = {2: [], 4: [], 6: [], 8: [], 10: []}
p01ValuesTrigger = {2: [], 4: [], 6: [], 8: [], 10: []}
p02ValuesTtfb = {2: [], 4: [], 6: [], 8: [], 10: []}
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
        for l in f:
            datapoint = json.loads(l)
            # print(datapoint)
            if datapoint["name"] == "time-to-fetch-ms":
                if p == 0.1:
                    p01ValuesTtfb[u].append(float(datapoint["measures"]["value"]) / 1000.0)
                if p == 0.2:
                    p02ValuesTtfb[u].append(float(datapoint["measures"]["value"]) / 1000.0)
            if datapoint["name"] == "unforwarded-search-counter":
                if p == 0.1:
                    p01ValuesTrigger[u].append(int(datapoint["measures"]["value"]))
                if p == 0.2:
                    p02ValuesTrigger[u].append(int(datapoint["measures"]["value"]))

# %%
fig, ax = plt.subplots()
pos = np.array(list(p01ValuesTtfb.keys()))

bpp01 = ax.boxplot(p01ValuesTtfb.values(), widths=0.3, patch_artist=True,
    showmeans=False, showfliers=True, sym="+",
    positions=pos-0.2,
    medianprops={"color": "white", "linewidth": 1.0},
    boxprops={"facecolor": "#ae4132", "edgecolor": "white",
                "linewidth": 0.5},
    whiskerprops={"color": "#ae4132", "linewidth": 1.5},
    capprops={"color": "#ae4132", "linewidth": 1.5})
bpp02 = ax.boxplot(p02ValuesTtfb.values(), widths=0.3, patch_artist=True,
    showmeans=False, showfliers=False, sym="+",
    positions=pos+0.2,
    medianprops={"color": "white", "linewidth": 1.0},
    boxprops={"facecolor": "#56517e", "edgecolor": "white",
                "linewidth": 0.5},
    whiskerprops={"color": "#56517e", "linewidth": 1.5},
    capprops={"color": "#56517e", "linewidth": 1.5})

plt.xticks(labels, labels)
ax.set_ylabel("TTFB (s)")
ax.set_xlabel("unforwarded search timer duration (s)")
ax.legend([bpp01["boxes"][0], bpp02["boxes"][0]], ["p=0.1", "p=0.2"], loc="upper right")

# plt.savefig("plots/" + NAME + "-ttfb.svg", format="svg")
plt.show()

# %%
# todo trigger count (as fraction)
