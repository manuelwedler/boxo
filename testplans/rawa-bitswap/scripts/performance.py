# %%
import os
import json
import matplotlib.pyplot as plt
import numpy as np

NAME = "performance"
FOLDER = "results/" + NAME
FOLDER_BASELINE = "results/baseline-" + NAME

labels = [0.05, 0.1, 0.2, 0.3]
n2Ttfbs = {"0.05": [], "0.1": [], "0.2": [], "0.3": []}
n4Ttfbs = {"0.05": [], "0.1": [], "0.2": [], "0.3": []}
n0Ttfbs = {"0.05": [], "0.1": [], "0.2": [], "0.3": []}

baselineTtfbs = []

for subdir, _, files in os.walk(FOLDER):
    if "results.out" not in files:
        continue
    peerNumber = subdir.split("/")[-1]
    dataDirName = subdir.split("/")[-3]
    nStr = dataDirName.split("_")[0]
    pStr = dataDirName.split("_")[1]
    n = int(nStr[2:-1])
    p = pStr[2:-1]
    with open(subdir + "/results.out") as f:
        for l in f:
            datapoint = json.loads(l)
            if datapoint["name"] == "time-to-fetch-ms":
                if n == 0:
                    n0Ttfbs[p].append(float(datapoint["measures"]["value"]) / 1000.0)
                if n == 2:
                    n2Ttfbs[p].append(float(datapoint["measures"]["value"]) / 1000.0)
                if n == 4:
                    n4Ttfbs[p].append(float(datapoint["measures"]["value"]) / 1000.0)

for subdir, _, files in os.walk(FOLDER_BASELINE):
    if "results.out" not in files:
        continue
    with open(subdir + "/results.out") as f:
        for l in f:
            datapoint = json.loads(l)
            if datapoint["name"] == "time-to-fetch-ms":
                baselineTtfbs.append(float(datapoint["measures"]["value"]) / 1000.0)


# %%
fig, ax = plt.subplots(figsize=[3.6, 4.8])
pos = np.arange(1, 5)

bpp01 = ax.boxplot(n2Ttfbs.values(), widths=0.2, patch_artist=True,
    showmeans=False, showfliers=True, sym="+",
    positions=pos-0.25,
    medianprops={"color": "white", "linewidth": 1.0},
    boxprops={"facecolor": "#ae4132", "edgecolor": "white",
                "linewidth": 0.5},
    whiskerprops={"color": "#ae4132", "linewidth": 1.5},
    capprops={"color": "#ae4132", "linewidth": 1.5})
bpp02 = ax.boxplot(n4Ttfbs.values(), widths=0.2, patch_artist=True,
    showmeans=False, showfliers=True, sym="+",
    positions=pos,
    medianprops={"color": "white", "linewidth": 1.0},
    boxprops={"facecolor": "#56517e", "edgecolor": "white",
                "linewidth": 0.5},
    whiskerprops={"color": "#56517e", "linewidth": 1.5},
    capprops={"color": "#56517e", "linewidth": 1.5})
bpp03 = ax.boxplot(n0Ttfbs.values(), widths=0.2, patch_artist=True,
    showmeans=False, showfliers=True, sym="+",
    positions=pos+0.25,
    medianprops={"color": "white", "linewidth": 1.0},
    boxprops={"facecolor": "#b46504", "edgecolor": "white",
                "linewidth": 0.5},
    whiskerprops={"color": "#b46504", "linewidth": 1.5},
    capprops={"color": "#b46504", "linewidth": 1.5})

plt.xticks([1,2,3,4], labels)
ax.set_ylabel("TTFB (s)")
ax.set_xlabel("proxy transition probability p")

# grid configuration
plt.yticks(np.arange(1,14,0.5), minor=True) 
plt.tick_params(which='minor', length=0)  
plt.grid(axis="y")
plt.grid(axis="y", which="minor")

ax.legend(
    [bpp01["boxes"][0], bpp02["boxes"][0], bpp03["boxes"][0]], 
    ["η=1", "η=2", "η=∆(G)"], 
    loc="upper right")

# plt.savefig("plots/" + NAME + "-ttfb.svg", format="svg")
plt.show()

# %%
fig, ax = plt.subplots(figsize=[3.6, 4.8])

bpp01 = ax.boxplot(baselineTtfbs, widths=0.1, patch_artist=True,
    showmeans=False, showfliers=True, sym="+",
    # positions=pos-0.25,
    medianprops={"color": "white", "linewidth": 1.0},
    boxprops={"facecolor": "#0e8088", "edgecolor": "white",
                "linewidth": 0.5},
    whiskerprops={"color": "#0e8088", "linewidth": 1.5},
    capprops={"color": "#0e8088", "linewidth": 1.5})

plt.xticks([], [])
ax.set_ylabel("TTFB (s)")

# grid configuration
plt.yticks(np.arange(1,10,0.5), minor=True)
plt.tick_params(which='minor', length=0)  
plt.grid(axis="y")
plt.grid(axis="y", which="minor")

ax.legend(
    [bpp01["boxes"][0]], 
    ["Baseline\nBitswap"], 
    loc="upper right")

# plt.savefig("plots/baseline-" + NAME + "-ttfb.svg", format="svg")
plt.show()

# %%

# print("MEANS")
# print(f"baseline: {np.mean(baselineTtfbs)}")
# for k, v in n2Ttfbs.items():
#     print(f"RaWa [η=1,p={k}]: {np.mean(v)}")
# for k, v in n4Ttfbs.items():
#     print(f"RaWa [η=2,p={k}]: {np.mean(v)}")
# for k, v in n0Ttfbs.items():
#     print(f"RaWa [η=∆(G),p={k}]: {np.mean(v)}")

# print()

print("MEDIAN")
print(f"baseline: {round(np.median(baselineTtfbs), 2)}")
for k, v in n2Ttfbs.items():
    print(f"RaWa [η=1,p={k}]: {round(np.median(v), 2)}")
for k, v in n4Ttfbs.items():
    print(f"RaWa [η=2,p={k}]: {round(np.median(v), 2)}")
for k, v in n0Ttfbs.items():
    print(f"RaWa [η=∆(G),p={k}]: {round(np.median(v), 2)}")


# %%

print("Q1 & Q3 TTTFB")
print(f"baseline: {round(np.quantile(baselineTtfbs, 0.25), 2)} - {round(np.quantile(baselineTtfbs, 0.75), 2)}")
for k, v in n2Ttfbs.items():
    print(f"RaWa [η=1,p={k}]: {round(np.quantile(v, 0.25), 2)} - {round(np.quantile(v, 0.75), 2)}")
for k, v in n4Ttfbs.items():
    print(f"RaWa [η=2,p={k}]: {round(np.quantile(v, 0.25), 2)} - {round(np.quantile(v, 0.75), 2)}")
for k, v in n0Ttfbs.items():
    print(f"RaWa [η=∆(G),p={k}]: {round(np.quantile(v, 0.25), 2)} - {round(np.quantile(v, 0.75), 2)}")

# %%

def maximum(v):
    q1 = np.quantile(v, 0.25)
    q3 = np.quantile(v, 0.75)
    iqr = q3 - q1
    return q3 + 1.5 * iqr

def minimum(v):
    q1 = np.quantile(v, 0.25)
    q3 = np.quantile(v, 0.75)
    iqr = q3 - q1
    return q1 - 1.5 * iqr

print("Q0 & Q4 TTTFB")
print(f"baseline: {round(minimum(baselineTtfbs), 2)} - {round(maximum(baselineTtfbs), 2)}")
for k, v in n2Ttfbs.items():
    print(f"RaWa [η=1,p={k}]: {round(minimum(v), 2)} - {round(maximum(v), 2)}")
for k, v in n4Ttfbs.items():
    print(f"RaWa [η=2,p={k}]: {round(minimum(v), 2)} - {round(maximum(v), 2)}")
for k, v in n0Ttfbs.items():
    print(f"RaWa [η=∆(G),p={k}]: {round(minimum(v), 2)} - {round(maximum(v), 2)}")


# %%
