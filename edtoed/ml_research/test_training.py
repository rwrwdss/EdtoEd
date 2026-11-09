import unittest
import numpy as np
import pandas as pd
from train_oulad import metrics, split_by_student, normalize_cohort, calibrate, choose_threshold, sigmoid


class ProtocolTests(unittest.TestCase):
    def test_calibration_reduces_brier_on_shifted_probabilities(self):
        logits = np.array([-2., -1., 0., 1., 2.])
        labels = np.array([0, 0, 0, 1, 1])
        a, c = calibrate(logits, labels)
        self.assertGreater(a, 0)
        self.assertLess(np.mean((sigmoid(a*logits+c)-labels)**2), np.mean((sigmoid(logits)-labels)**2))

    def test_threshold_precision_floor(self):
        y = np.array([0, 0, 1, 1, 1])
        p = np.array([.1, .2, .3, .7, .8])
        threshold, selected = choose_threshold(y, p)
        self.assertGreaterEqual(selected['precision'], .6)
        self.assertLessEqual(threshold, .3)

    def test_zero_inclusive_median_and_unscorable_groups(self):
        frame = pd.DataFrame({'code_module': ['A']*4+['B']*2+['C'],
                              'code_presentation': ['2013J']*7,
                              'clicks': [0, 0, 10, 30, 0, 0, 20]})
        result = normalize_cohort(frame)
        self.assertEqual(result.median_clicks[:4].tolist(), [5]*4)
        self.assertEqual(result.relative_clicks[:4].tolist(), [0,0,2,6])
        self.assertFalse(result.scorable[4:].any())
        self.assertTrue(result.relative_clicks[4:].isna().all())

    def test_tied_auc(self):
        result = metrics(np.array([0, 1, 0, 1]), np.full(4, .5))
        self.assertEqual(result['roc_auc'], .5)
        self.assertEqual(result['brier'], .25)

    def test_temporal_student_isolation(self):
        data = pd.DataFrame({'id_student': [1, 1, 2, 2, 3],
                             'code_presentation': ['2013J', '2014B', '2014B', '2014J', '2014J']})
        train, valid, test = split_by_student(data)
        self.assertEqual(train.id_student.tolist(), [1])
        self.assertEqual(valid.id_student.tolist(), [2])
        self.assertEqual(test.id_student.tolist(), [3])


if __name__ == '__main__':
    unittest.main()
