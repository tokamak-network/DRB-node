import matplotlib.pyplot as plt
import numpy as np

def plot_data(T_values, results, labels, styles, colors, title, x_label, y_label, y_scale='linear'):
    # Adjust figure size
    plt.figure(figsize=(10, 6))

    # Iterate over the data and plot with different styles
    for result, label, style, color in zip(results, labels, styles, colors):
        plt.plot(T_values, result, marker='o', linestyle=style, color=color, label=label)

    # Set x and y axis labels
    plt.xlabel(x_label, fontsize=14)
    plt.ylabel(y_label, fontsize=14)

    # Set the title
    plt.title(title, fontsize=16)

    # Customize x-axis ticks
    plt.xticks(T_values, labels=[f'{i}' for i in T_values])

    # Enable grid for better readability
    plt.grid(True, which='both', linestyle='--', linewidth=0.5, color='grey', alpha=0.7)

    # Set the legend position
    plt.legend(loc='upper left')

    # Set y-axis scale ('linear' or 'log' supported)
    plt.yscale(y_scale)

    # Display the plot
    plt.show()

# Example data
T_values = np.array([1, 2, 3, 5, 10, 15, 20, 25, 30])
results = [
    np.array([70, 85, 87, 92, 112, 152, 199, 240, 279]),
]
labels = ['5 Operators']
styles = ['-']
colors = ['blue']

# Call the function (y-axis scale can be set to 'log' if needed)
plot_data(T_values, results, labels, styles, colors, 'Execution time of DRB node', 'Request of Random words', 'Time(s)', y_scale='linear')
