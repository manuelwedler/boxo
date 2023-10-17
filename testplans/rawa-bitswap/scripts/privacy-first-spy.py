# %%
import os
import json
import matplotlib.pyplot as plt
import numpy as np

NAME = "privacy-first-spy"
FOLDER = "results/" + NAME
FOLDER_BASELINE = "results/baseline-" + NAME

labels = [0.05, 0.1, 0.2, 0.3]
n2Precisions = {"0.05": [], "0.1": [], "0.2": [], "0.3": []}
n4Precisions = {"0.05": [], "0.1": [], "0.2": [], "0.3": []}
n0Precisions = {"0.05": [], "0.1": [], "0.2": [], "0.3": []}
n2Recalls = {"0.05": [], "0.1": [], "0.2": [], "0.3": []}
n4Recalls = {"0.05": [], "0.1": [], "0.2": [], "0.3": []}
n0Recalls = {"0.05": [], "0.1": [], "0.2": [], "0.3": []}

baselinePrecisions = []
baselineRecalls = []

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
            if datapoint["name"] == "first-spy-estimator-precision":
                if n == 0:
                    n0Precisions[p].append(float(datapoint["measures"]["value"]))
                if n == 2:
                    n2Precisions[p].append(float(datapoint["measures"]["value"]))
                if n == 4:
                    n4Precisions[p].append(float(datapoint["measures"]["value"]))
            if datapoint["name"] == "first-spy-estimator-recall":
                if n == 0:
                    n0Recalls[p].append(float(datapoint["measures"]["value"]))
                if n == 2:
                    n2Recalls[p].append(float(datapoint["measures"]["value"]))
                if n == 4:
                    n4Recalls[p].append(float(datapoint["measures"]["value"]))

for subdir, _, files in os.walk(FOLDER_BASELINE):
    if "results.out" not in files:
        continue
    with open(subdir + "/results.out") as f:
        for l in f:
            datapoint = json.loads(l)
            if datapoint["name"] == "first-spy-estimator-precision":
                baselinePrecisions.append(float(datapoint["measures"]["value"]))
            if datapoint["name"] == "first-spy-estimator-recall":
                baselineRecalls.append(float(datapoint["measures"]["value"]))


# %%
fig, ax = plt.subplots(figsize=[3.6, 4.8])
pos = np.arange(1, 5)

bpp01 = ax.boxplot(n2Precisions.values(), widths=0.2, patch_artist=True,
    showmeans=False, showfliers=True, sym="+",
    positions=pos-0.25,
    medianprops={"color": "white", "linewidth": 1.0},
    boxprops={"facecolor": "#ae4132", "edgecolor": "white",
                "linewidth": 0.5},
    whiskerprops={"color": "#ae4132", "linewidth": 1.5},
    capprops={"color": "#ae4132", "linewidth": 1.5})
bpp02 = ax.boxplot(n4Precisions.values(), widths=0.2, patch_artist=True,
    showmeans=False, showfliers=True, sym="+",
    positions=pos,
    medianprops={"color": "white", "linewidth": 1.0},
    boxprops={"facecolor": "#56517e", "edgecolor": "white",
                "linewidth": 0.5},
    whiskerprops={"color": "#56517e", "linewidth": 1.5},
    capprops={"color": "#56517e", "linewidth": 1.5})
bpp03 = ax.boxplot(n0Precisions.values(), widths=0.2, patch_artist=True,
    showmeans=False, showfliers=True, sym="+",
    positions=pos+0.25,
    medianprops={"color": "white", "linewidth": 1.0},
    boxprops={"facecolor": "#b46504", "edgecolor": "white",
                "linewidth": 0.5},
    whiskerprops={"color": "#b46504", "linewidth": 1.5},
    capprops={"color": "#b46504", "linewidth": 1.5})

plt.xticks([1,2,3,4], labels)
ax.set_ylabel("precision D of FSE")
ax.set_xlabel("proxy transition probability p")

# grid configuration
plt.yticks(np.arange(0.05,0.5,0.05), minor=True) 
plt.tick_params(which='minor', length=0)  
plt.grid(axis="y")
plt.grid(axis="y", which="minor")

ax.legend(
    [bpp01["boxes"][0], bpp02["boxes"][0], bpp03["boxes"][0]], 
    ["η=1", "η=2", "η=∆(G)"], 
    loc="lower left")

# plt.savefig("plots/" + NAME + "-precision.svg", format="svg")
plt.show()

# %%
fig, ax = plt.subplots(figsize=[3.6, 4.8])
pos = np.arange(1, 5)

bpp01 = ax.boxplot(n2Recalls.values(), widths=0.2, patch_artist=True,
    showmeans=False, showfliers=True, sym="+",
    positions=pos-0.25,
    medianprops={"color": "white", "linewidth": 1.0},
    boxprops={"facecolor": "#ae4132", "edgecolor": "white",
                "linewidth": 0.5},
    whiskerprops={"color": "#ae4132", "linewidth": 1.5},
    capprops={"color": "#ae4132", "linewidth": 1.5})
bpp02 = ax.boxplot(n4Recalls.values(), widths=0.2, patch_artist=True,
    showmeans=False, showfliers=True, sym="+",
    positions=pos,
    medianprops={"color": "white", "linewidth": 1.0},
    boxprops={"facecolor": "#56517e", "edgecolor": "white",
                "linewidth": 0.5},
    whiskerprops={"color": "#56517e", "linewidth": 1.5},
    capprops={"color": "#56517e", "linewidth": 1.5})
bpp03 = ax.boxplot(n0Recalls.values(), widths=0.2, patch_artist=True,
    showmeans=False, showfliers=True, sym="+",
    positions=pos+0.25,
    medianprops={"color": "white", "linewidth": 1.0},
    boxprops={"facecolor": "#b46504", "edgecolor": "white",
                "linewidth": 0.5},
    whiskerprops={"color": "#b46504", "linewidth": 1.5},
    capprops={"color": "#b46504", "linewidth": 1.5})

plt.xticks([1,2,3,4], labels)
ax.set_ylabel("recall R of FSE")
ax.set_xlabel("proxy transition probability p")

# grid configuration
plt.yticks(np.arange(0.1,0.5,0.05), minor=True)
plt.tick_params(which='minor', length=0)
plt.grid(axis="y")
plt.grid(axis="y", which="minor")

ax.legend(
    [bpp01["boxes"][0], bpp02["boxes"][0], bpp03["boxes"][0]], 
    ["η=1", "η=2", "η=∆(G)"], 
    loc="lower left")

# plt.savefig("plots/" + NAME + "-recall.svg", format="svg")
plt.show()

# %%
fig, ax = plt.subplots(figsize=[3.6, 4.8])

bpp01 = ax.boxplot(baselinePrecisions, widths=0.1, patch_artist=True,
    showmeans=False, showfliers=True, sym="+",
    medianprops={"color": "white", "linewidth": 1.0},
    boxprops={"facecolor": "#0e8088", "edgecolor": "white",
                "linewidth": 0.5},
    whiskerprops={"color": "#0e8088", "linewidth": 1.5},
    capprops={"color": "#0e8088", "linewidth": 1.5})

plt.xticks([], [])
ax.set_ylabel("precision D of FSE")

# grid configuration
plt.yticks(np.arange(0.5,0.8,0.05), minor=True)
plt.tick_params(which='minor', length=0)  
plt.grid(axis="y")
plt.grid(axis="y", which="minor")

ax.legend(
    [bpp01["boxes"][0]], 
    ["Baseline\nBitswap"], 
    loc="upper right")

# plt.savefig("plots/baseline-" + NAME + "-precision.svg", format="svg")
plt.show()

# %%
fig, ax = plt.subplots(figsize=[3.6, 4.8])

bpp01 = ax.boxplot(baselineRecalls, widths=0.1, patch_artist=True,
    showmeans=False, showfliers=True, sym="+",
    medianprops={"color": "white", "linewidth": 1.0},
    boxprops={"facecolor": "#0e8088", "edgecolor": "white",
                "linewidth": 0.5},
    whiskerprops={"color": "#0e8088", "linewidth": 1.5},
    capprops={"color": "#0e8088", "linewidth": 1.5})

plt.xticks([], [])
ax.set_ylabel("recall R of FSE")

# grid configuration
plt.yticks(np.arange(0.55,0.8,0.05), minor=True)
plt.tick_params(which='minor', length=0)  
plt.grid(axis="y")
plt.grid(axis="y", which="minor")

ax.legend(
    [bpp01["boxes"][0]], 
    ["Baseline\nBitswap"], 
    loc="upper right")

# plt.savefig("plots/baseline-" + NAME + "-recall.svg", format="svg")
plt.show()

# %%

print("MEANS PRECISION FSE")
print(f"baseline: {np.mean(baselinePrecisions)}")
for k, v in n2Precisions.items():
    print(f"RaWa [η=1,p={k}]: {np.mean(v)}")
for k, v in n4Precisions.items():
    print(f"RaWa [η=2,p={k}]: {np.mean(v)}")
for k, v in n0Precisions.items():
    print(f"RaWa [η=∆(G),p={k}]: {np.mean(v)}")

print()

print("MEDIANS PRECISION FSE")
print(f"baseline: {np.median(baselinePrecisions)}")
for k, v in n2Precisions.items():
    print(f"RaWa [η=1,p={k}]: {np.median(v)}")
for k, v in n4Precisions.items():
    print(f"RaWa [η=2,p={k}]: {np.median(v)}")
for k, v in n0Precisions.items():
    print(f"RaWa [η=∆(G),p={k}]: {np.median(v)}")

# %%

print("MEANS RECALL FSE")
print(f"baseline: {np.mean(baselineRecalls)}")
for k, v in n2Recalls.items():
    print(f"RaWa [η=1,p={k}]: {np.mean(v)}")
for k, v in n4Recalls.items():
    print(f"RaWa [η=2,p={k}]: {np.mean(v)}")
for k, v in n0Recalls.items():
    print(f"RaWa [η=∆(G),p={k}]: {np.mean(v)}")

print()

print("MEDIANS RECALL FSE")
print(f"baseline: {np.median(baselineRecalls)}")
for k, v in n2Recalls.items():
    print(f"RaWa [η=1,p={k}]: {np.median(v)}")
for k, v in n4Recalls.items():
    print(f"RaWa [η=2,p={k}]: {np.median(v)}")
for k, v in n0Recalls.items():
    print(f"RaWa [η=∆(G),p={k}]: {np.median(v)}")

# %%
