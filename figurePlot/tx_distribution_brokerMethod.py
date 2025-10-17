import pandas as pd 
import seaborn as sns
import matplotlib.pyplot as plt
from matplotlib import font_manager as fm

# 中文字体路径（Windows）
font_path = 'C:/Windows/Fonts/simhei.ttf'  # 可换成 msyh.ttc 等
my_font = fm.FontProperties(fname=font_path)

# 读取CSV
file_path = './expTest/result/supervisor_measureOutput/Tx_Details.csv'
df = pd.read_csv(file_path)

latency_column = 'Confirmed latency of this tx (ms)'
timestamp_column = 'Broker1 Tx commit timestamp (not a broker tx -> nil)'

latency_data_1 = df[latency_column]
latency_data_2 = df[df[timestamp_column].notna()][latency_column]
latency_data_3 = df[df[timestamp_column].isna()][latency_column]

# 绘图
sns.set(style='whitegrid')
plt.figure(figsize=(10, 6))
density_kwargs = {'lw': 2, 'alpha': 0.7}

sns.kdeplot(latency_data_1, color='blue', label='所有交易', **density_kwargs)
sns.kdeplot(latency_data_2, color='orange', label='broker处理的跨片交易', **density_kwargs)
sns.kdeplot(latency_data_3, color='green', label='片内交易', **density_kwargs)

# 设置中文及字体大小
plt.title('交易确认延迟分布 (ms)', fontproperties=my_font, fontsize=20)
plt.xlabel('交易确认延迟 (ms)', fontproperties=my_font, fontsize=18)
plt.ylabel('密度', fontproperties=my_font, fontsize=18)
plt.legend(prop=my_font, fontsize=18)

plt.tight_layout()
plt.show()
