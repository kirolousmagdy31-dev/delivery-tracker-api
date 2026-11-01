CREATE DATABASE IF NOT EXISTS package_tracking CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE package_tracking;

CREATE TABLE IF NOT EXISTS users (
  id            BIGINT AUTO_INCREMENT PRIMARY KEY,
  name          VARCHAR(100) NOT NULL,
  email         VARCHAR(150) NOT NULL UNIQUE,
  phone         VARCHAR(20)  NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  role          ENUM('user','courier','admin') NOT NULL DEFAULT 'user',
  created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS orders (
  id               BIGINT AUTO_INCREMENT PRIMARY KEY,
  user_id          BIGINT NOT NULL,
  courier_id       BIGINT NULL,
  pickup_location  VARCHAR(255) NOT NULL,
  dropoff_location VARCHAR(255) NOT NULL,
  package_details  TEXT NOT NULL,
  delivery_time    DATETIME NOT NULL,
  delivery_fee     DECIMAL(10,2) NOT NULL DEFAULT 0,
  status ENUM('pending','assigned','accepted','picked_up','in_transit','delivered','cancelled') NOT NULL DEFAULT 'pending',
  created_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX idx_orders_user (user_id),
  INDEX idx_orders_courier (courier_id),
  INDEX idx_orders_status (status),
  CONSTRAINT fk_orders_user    FOREIGN KEY (user_id)    REFERENCES users(id),
  CONSTRAINT fk_orders_courier FOREIGN KEY (courier_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS order_status_history (
  id         BIGINT AUTO_INCREMENT PRIMARY KEY,
  order_id   BIGINT NOT NULL,
  status     VARCHAR(20) NOT NULL,
  changed_by BIGINT NOT NULL,
  note       VARCHAR(255) NOT NULL DEFAULT '',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_hist_order (order_id),
  CONSTRAINT fk_hist_order FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE CASCADE
);
