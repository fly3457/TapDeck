本目录存放接收端内嵌的精简 LGPL ffmpeg（ffmpeg.exe）及其清单 manifest.json，
由 scripts/build-ffmpeg.ps1 从固定版本的上游源码构建生成。
缺少 ffmpeg.exe 时接收端不声明 video 能力，手机端的视频模式不可用。
