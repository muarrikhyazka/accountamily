FROM python:3.11-slim
WORKDIR /app
ENV TZ=Asia/Jakarta
RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY app.py schema.sql seed_demo.py .
COPY react/dist /app/react_dist
RUN mkdir -p /app/data
CMD ["gunicorn", "-w", "2", "-b", "0.0.0.0:5000", "--timeout", "30", "app:app"]
