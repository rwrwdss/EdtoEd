import json
import sys
import numpy as np
import onnxruntime as ort

options = ort.SessionOptions()
options.intra_op_num_threads = 1
options.inter_op_num_threads = 1
session = ort.InferenceSession(sys.argv[1], sess_options=options, providers=['CPUExecutionProvider'])
inputs = session.get_inputs()
if len(inputs) != 1 or inputs[0].name != 'float_input' or inputs[0].type != 'tensor(float)' or inputs[0].shape[-1] != 3:
    raise ValueError('Expected float_input float32 [N,3]')

def predict(values):
    x = np.asarray([values], dtype=np.float32)
    if x.shape != (1, 3) or not np.isfinite(x).all():
        raise ValueError('Invalid input')
    probabilities = session.run(['probabilities'], {'float_input': x})[0]
    risk = float(probabilities[0][1])
    if not np.isfinite(risk) or not 0 <= risk <= 1:
        raise ValueError('Invalid probability')
    return risk

predict([1, 0.5, 0])
print('ready', flush=True)
for line in sys.stdin:
    print(json.dumps(predict(json.loads(line)), allow_nan=False), flush=True)
