# 开发指南

## 本地 PyInstaller 打包与测试

在推送到 GitHub Actions 之前，可以在本地构建单文件可执行程序来验证打包是否正确，尽早发现隐藏依赖或数据文件缺失等问题。

### 前置条件

```bash
# 确保在项目根目录，且已激活 venv
pip install pyinstaller
pip install -r requirements.txt
```

### 构建完整二进制

```bash
.venv/bin/python -m PyInstaller --clean --onefile \
    --additional-hooks-dir hooks \
    --collect-all rich \
    --hidden-import litellm \
    --collect-data litellm \
    --name MailBot main.py
```

可执行文件位于 `dist/MailBot`，运行：

```bash
./dist/MailBot --help
./dist/MailBot --headless -c config.json
```
