@echo off
chcp 65001 >nul
cd /d "%~dp0"
where py >nul 2>nul
if %errorlevel%==0 (
  py -3 a9-wifi-setup.py
) else (
  python a9-wifi-setup.py
)
if %errorlevel% neq 0 pause
