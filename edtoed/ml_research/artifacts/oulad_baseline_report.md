# OULAD baseline

```json
{
  "dataset": "OULAD",
  "target": "Fail or Withdrawn",
  "window_days": 28,
  "protocol": "2013 train / unseen students 2014B validation / unseen students 2014J test; active on day 28; submissions <=28; banked excluded",
  "limitations": "OULAD previously explored; this is not a new independent dataset. Scores assumed available at submission. Cohort medians use positive-activity participants at day 28. Production normalization still pending.",
  "features": [
    "relative_clicks",
    "test_score",
    "deadline_delay"
  ],
  "rows": 27515,
  "students": 24826,
  "splits": {
    "train": {
      "n": 11797,
      "positive_rate": 0.4434178180893448,
      "accuracy": 0.6463507671441892,
      "precision": 0.5963603275705186,
      "recall": 0.62645765627987,
      "f1": 0.6110385977997389,
      "roc_auc": 0.7037803231782132,
      "brier": 0.21783126628582053,
      "tp": 3277,
      "tn": 4348,
      "fp": 2218,
      "fn": 1954
    },
    "validation": {
      "n": 5832,
      "positive_rate": 0.4660493827160494,
      "accuracy": 0.6454046639231824,
      "precision": 0.6071193144363876,
      "recall": 0.6777041942604857,
      "f1": 0.6404728789986092,
      "roc_auc": 0.7048702529297535,
      "brier": 0.21974092436148412,
      "tp": 1842,
      "tn": 1922,
      "fp": 1192,
      "fn": 876
    },
    "test": {
      "n": 7929,
      "positive_rate": 0.3836549375709421,
      "accuracy": 0.6210114768571068,
      "precision": 0.504633107938893,
      "recall": 0.6623931623931624,
      "f1": 0.5728500355366027,
      "roc_auc": 0.6912245344388708,
      "brier": 0.22634299262743166,
      "tp": 2015,
      "tn": 2909,
      "fp": 1978,
      "fn": 1027
    }
  },
  "normalization_mean": [
    0.46344180264892926,
    0.5450864626599866,
    0.047244429146223246
  ],
  "normalization_scale": [
    0.29326940131367574,
    0.35048962008972173,
    0.14932045567739316
  ],
  "weights": [
    -0.546252252462539,
    -0.357677092833466,
    0.23744147320126588
  ],
  "bias": -0.02543467308809272
}
```
