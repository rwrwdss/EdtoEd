# OULAD baseline

```json
{
  "dataset": "OULAD",
  "target": "Fail or Withdrawn",
  "window_days": 28,
  "features": [
    "relative_clicks",
    "test_score",
    "deadline_delay"
  ],
  "rows": 32593,
  "students": 28785,
  "splits": {
    "train": {
      "n": 22866,
      "positive_rate": 0.5282515525233972,
      "accuracy": 0.6997725881221026,
      "precision": 0.727367870225013,
      "recall": 0.6904545078234953,
      "f1": 0.7084306646846465,
      "roc_auc": 0.7768341592043537
    },
    "validation": {
      "n": 4875,
      "positive_rate": 0.5263589743589744,
      "accuracy": 0.7017435897435897,
      "precision": 0.7234726688102894,
      "recall": 0.7014809041309431,
      "f1": 0.7123070834982191,
      "roc_auc": 0.7776368657397078
    },
    "test": {
      "n": 4852,
      "positive_rate": 0.5282357790601814,
      "accuracy": 0.7079554822753503,
      "precision": 0.7299357945425361,
      "recall": 0.7097151775263363,
      "f1": 0.7196834817012858,
      "roc_auc": 0.782541551845013
    }
  },
  "normalization_mean": [
    0.40877267643003556,
    0.465458541065336,
    0.08866135359019099
  ],
  "normalization_scale": [
    0.31280640058336356,
    0.3878279750499345,
    0.21186958275335604
  ],
  "weights": [
    -0.845284388204394,
    -0.4903825027679524,
    0.2540707954620673
  ],
  "bias": 0.02958766333791572
}
```
