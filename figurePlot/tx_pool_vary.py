import os
import pandas as pd
import matplotlib.pyplot as plt
from matplotlib import font_manager as fm

# 加载中文字体
font_path = 'C:/Windows/Fonts/simhei.ttf'  # 可替换为其他中文字体路径
my_font = fm.FontProperties(fname=font_path)

# CSV目录
directory = './expTest/result/pbft_shardNum=2'
csv_files = [f for f in os.listdir(directory) if f.endswith('.csv')]

plt.figure(figsize=(10, 6))

x = 1  # 去除文件名后缀
for csv_file in csv_files:
    file_path = os.path.join(directory, csv_file)
    df = pd.read_csv(file_path)

    block_height = df['Block Height']
    tx_pool_size = df['TxPool Size']

    parts = csv_file.split('_')
    file_name = '_'.join(parts[:-x]) if len(parts) > x else csv_file

    plt.plot(block_height, tx_pool_size, label=file_name)

# 设置中文标签和字体大小
plt.title('交易池大小随区块高度变化', fontproperties=my_font, fontsize=20)
plt.xlabel('区块高度', fontproperties=my_font, fontsize=18)
plt.ylabel('交易池大小', fontproperties=my_font, fontsize=18)
plt.legend(prop=my_font, fontsize=18)

plt.grid(True)
plt.tight_layout()
plt.show()
