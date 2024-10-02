import matplotlib.pyplot as plt
import numpy as np

def plot_data(T_values, results, labels, styles, colors, title, x_label, y_label):
    plt.figure(figsize=(10, 6))
    for result, label, style, color in zip(results, labels, styles, colors):
        plt.plot(T_values, result, marker='o', linestyle=style, color=color, label=label)
    plt.xlabel(x_label, fontsize=14)
    plt.ylabel(y_label, fontsize=14)
    plt.title(title, fontsize=16)
    plt.xticks(T_values, labels=[f'{i}' for i in T_values])
    plt.grid(True, which='both', linestyle='--', linewidth=0.5, color='grey', alpha=0.7)
    plt.legend(loc='upper left')
    plt.yscale('linear')
    plt.show()

# example
T_values = np.array([1, 2, 3, 5, 10, 15, 20, 25, 30])
results = [
    np.array([70, 85, 87, 92, 112, 152, 199, 240, 279]),

]
labels = ['5 Operators']
styles = ['-']
colors = ['blue']

plot_data(T_values, results, labels, styles, colors, 'Execution time of DRB node', 'Request of Random words', 'Time(s)')
